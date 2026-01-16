/**
 * Weave Compute - Request Processing Pipeline Implementation
 *
 * This module bridges protocol requests and the SD wrapper. It handles
 * parameter conversion, image generation, and error mapping.
 *
 * Safety principles:
 * - All pointers validated before use
 * - All allocations checked
 * - All errors mapped to appropriate status codes
 * - Image data ownership clearly documented
 */

#define _POSIX_C_SOURCE 199309L
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include "weave/generate.h"

/**
 * Convert protocol request to SD wrapper generation parameters.
 *
 * SD 3.5 uses a single prompt for all three encoders (CLIP-L, CLIP-G, T5).
 * The protocol allows different prompts per encoder, but for simplicity
 * we use the CLIP-L prompt as the main prompt.
 *
 * @param req     Protocol request
 * @param params  Output SD wrapper parameters
 * @param prompt  Output buffer for null-terminated prompt string
 * @param prompt_buf_size  Size of prompt buffer
 * @return        ERR_NONE on success, error code on failure
 */
static error_code_t convert_request_params(const sd35_generate_request_t *req,
                                            sd_wrapper_gen_params_t *params,
                                            char *prompt,
                                            size_t prompt_buf_size) {
    if (req == NULL || params == NULL || prompt == NULL) {
        return ERR_INTERNAL;
    }

    if (req->prompt_data == NULL) {
        return ERR_INVALID_PROMPT;
    }

    if (req->clip_l_length == 0 || req->clip_l_length >= prompt_buf_size) {
        return ERR_INVALID_PROMPT;
    }

    if (req->clip_l_offset + req->clip_l_length > req->prompt_data_len) {
        return ERR_INVALID_PROMPT;
    }

    memcpy(prompt, req->prompt_data + req->clip_l_offset, req->clip_l_length);
    prompt[req->clip_l_length] = '\0';

    sd_wrapper_gen_params_init(params);
    params->prompt = prompt;
    params->negative_prompt = NULL;
    params->width = req->width;
    params->height = req->height;
    params->steps = req->steps;
    params->cfg_scale = req->cfg_scale;
    params->seed = (int64_t)req->seed;
    params->clip_skip = 0;

    return ERR_NONE;
}

/**
 * Map SD wrapper error to protocol error code and status.
 *
 * @param sd_err  SD wrapper error code
 * @param status  Output status code (400 or 500)
 * @return        Protocol error code
 */
static error_code_t map_sd_error(sd_wrapper_error_t sd_err, uint32_t *status) {
    switch (sd_err) {
    case SD_WRAPPER_OK:
        *status = STATUS_OK;
        return ERR_NONE;

    case SD_WRAPPER_ERR_INVALID_PARAM:
        *status = STATUS_BAD_REQUEST;
        return ERR_INVALID_PROMPT;

    case SD_WRAPPER_ERR_OUT_OF_MEMORY:
        *status = STATUS_INTERNAL_SERVER_ERROR;
        return ERR_OUT_OF_MEMORY;

    case SD_WRAPPER_ERR_GPU_ERROR:
        *status = STATUS_INTERNAL_SERVER_ERROR;
        return ERR_GPU_ERROR;

    case SD_WRAPPER_ERR_MODEL_NOT_FOUND:
    case SD_WRAPPER_ERR_MODEL_CORRUPT:
    case SD_WRAPPER_ERR_INIT_FAILED:
    case SD_WRAPPER_ERR_GENERATION_FAILED:
    default:
        *status = STATUS_INTERNAL_SERVER_ERROR;
        return ERR_INTERNAL;
    }
}

/**
 * Get current time in milliseconds.
 *
 * @return  Current time in milliseconds since epoch
 */
static uint64_t get_time_ms(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000 + (uint64_t)ts.tv_nsec / 1000000;
}

/**
 * Process a generation request and produce a response.
 *
 * This function orchestrates the complete generation pipeline:
 * 1. Validates protocol request parameters
 * 2. Converts protocol parameters to SD wrapper format
 * 3. Calls SD wrapper to generate image
 * 4. Builds protocol response with image data
 * 5. Maps errors to appropriate status codes
 *
 * Error mapping:
 * - Invalid dimensions/steps/cfg → STATUS_BAD_REQUEST (400)
 * - Invalid prompt → STATUS_BAD_REQUEST (400)
 * - Model not loaded → STATUS_INTERNAL_SERVER_ERROR (500)
 * - GPU/OOM errors → STATUS_INTERNAL_SERVER_ERROR (500)
 *
 * @param ctx   SD wrapper context (must not be NULL, must be initialized)
 * @param req   Decoded protocol request (borrowed, not modified)
 * @param resp  Output response structure (populated on success)
 * @return      ERR_NONE on success, error code on failure
 *
 * @note On success, resp->image_data is allocated and must be freed by caller
 * @note On failure, resp is unchanged (no cleanup needed)
 * @note req->prompt_data must remain valid during this call
 * @note This function is NOT thread-safe (ctx is single-threaded)
 */
/*
 * Track whether a generation has been performed.
 *
 * stable-diffusion.cpp requires a context reset between generations, but
 * NOT before the first generation. The initially created context works
 * correctly, but recreated contexts from sd_wrapper_reset() may have
 * subtle differences that cause crashes.
 */
static bool generation_performed = false;

error_code_t process_generate_request(sd_wrapper_ctx_t *ctx,
                                       const sd35_generate_request_t *req,
                                       sd35_generate_response_t *resp) {
    if (ctx == NULL || req == NULL || resp == NULL) {
        return ERR_INTERNAL;
    }

    char prompt[SD35_MAX_PROMPT_LENGTH + 1];
    sd_wrapper_gen_params_t params;
    error_code_t err;
    sd_wrapper_error_t sd_err;

    err = convert_request_params(req, &params, prompt, sizeof(prompt));
    if (err != ERR_NONE) {
        return err;
    }

    /*
     * WORKAROUND: Reset SD context between generations to avoid segfault.
     *
     * stable-diffusion.cpp has a bug where GGML compute buffers are not
     * properly freed between generate_image() calls on the same context.
     * This causes segfaults on subsequent generations with different prompt
     * lengths. Resetting the context ensures clean state.
     *
     * Important: Only reset AFTER the first generation. The initially created
     * context works correctly, but we must reset before subsequent generations.
     *
     * Performance impact: ~2-3 seconds model reload per generation (after first).
     * This should be removed once the upstream bug is fixed.
     */
    if (generation_performed) {
        sd_err = sd_wrapper_reset(ctx);
        if (sd_err != SD_WRAPPER_OK) {
            return ERR_INTERNAL;
        }
    }

    sd_wrapper_image_t image;
    memset(&image, 0, sizeof(image));

    uint64_t start_time = get_time_ms();
    sd_err = sd_wrapper_generate(ctx, &params, &image);
    uint64_t end_time = get_time_ms();

    uint32_t status;
    err = map_sd_error(sd_err, &status);
    if (err != ERR_NONE) {
        return err;
    }

    if (image.data == NULL) {
        return ERR_INTERNAL;
    }

    /* Validate image dimensions match request */
    if (image.width != req->width || image.height != req->height) {
        sd_wrapper_free_image(&image);
        return ERR_INTERNAL;
    }

    /* Validate dimensions are within protocol bounds */
    if (image.width < SD35_MIN_DIMENSION || image.width > SD35_MAX_DIMENSION ||
        image.width % 64 != 0) {
        sd_wrapper_free_image(&image);
        return ERR_INTERNAL;
    }

    if (image.height < SD35_MIN_DIMENSION || image.height > SD35_MAX_DIMENSION ||
        image.height % 64 != 0) {
        sd_wrapper_free_image(&image);
        return ERR_INTERNAL;
    }

    /* Validate channel count (RGB or RGBA) */
    if (image.channels != 3 && image.channels != 4) {
        sd_wrapper_free_image(&image);
        return ERR_INTERNAL;
    }

    /* Validate image data size fits in uint32_t (protocol constraint) */
    if (image.data_size > UINT32_MAX) {
        sd_wrapper_free_image(&image);
        return ERR_INTERNAL;
    }

    uint64_t generation_time = end_time - start_time;
    /* Clamp generation time to UINT32_MAX (~49 days, acceptable limit) */
    if (generation_time > UINT32_MAX) {
        generation_time = UINT32_MAX;
    }

    resp->request_id = req->request_id;
    resp->status = STATUS_OK;
    resp->generation_time_ms = (uint32_t)generation_time;
    resp->image_width = image.width;
    resp->image_height = image.height;
    resp->channels = image.channels;
    resp->image_data_len = (uint32_t)image.data_size;
    resp->image_data = image.data;

    /* Mark that a generation has been performed for reset logic */
    generation_performed = true;

    return ERR_NONE;
}

/**
 * Free response image data allocated by process_generate_request().
 *
 * This is a convenience wrapper around sd_wrapper_free_image().
 * Safe to call with NULL or already-freed image data.
 *
 * @param resp  Response structure (image_data will be freed and set to NULL)
 */
void free_generate_response(sd35_generate_response_t *resp) {
    if (resp == NULL) {
        return;
    }

    if (resp->image_data != NULL) {
        sd_wrapper_image_t image;
        image.width = resp->image_width;
        image.height = resp->image_height;
        image.channels = resp->channels;
        image.data = (uint8_t *)resp->image_data;
        image.data_size = resp->image_data_len;

        sd_wrapper_free_image(&image);

        resp->image_data = NULL;
        resp->image_data_len = 0;
    }
}
