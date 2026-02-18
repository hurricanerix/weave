/**
 * Wrapper implementation for stable-diffusion.cpp integration.
 *
 * This file bridges our C99 interface to stable-diffusion.cpp's C API.
 * stable-diffusion.h provides an extern "C" interface, so no C++ needed here.
 */

#include "weave/sd_wrapper.h"

#include <stdlib.h>
#include <string.h>
#include <limits.h>
#include <stdbool.h>
#include <stdio.h>
#include <errno.h>
#include <unistd.h>

/* Include stable-diffusion.cpp C API */
#include "stable-diffusion.h"

/* Include protocol for event encoding */
#include "weave/protocol.h"

/** Maximum error message length */
#define SD_WRAPPER_ERROR_MSG_SIZE 256

/**
 * Internal context structure.
 * Holds stable-diffusion.cpp context and error state.
 */
struct sd_wrapper_ctx {
    sd_ctx_t* sd_ctx;                      /* stable-diffusion.cpp context */
    char error_msg[SD_WRAPPER_ERROR_MSG_SIZE]; /* Last error message */
    sd_wrapper_config_t config;            /* Configuration used to create context */
    sd_wrapper_callback_ctx_t callback_ctx; /* Callback context (socket fd, request_id) */
    bool callbacks_enabled;                /* Whether callbacks are enabled */
};

/* Forward declarations */
static void sd_wrapper_log_callback(enum sd_log_level_t level,
                                     const char* text,
                                     void* data);
static void sd_wrapper_progress_callback(int step, int steps, float time, void* data);
static void sd_wrapper_preview_callback(int step, int frame_count,
                                          sd_image_t* frames, bool is_noisy, void* data);

/**
 * Initialize wrapper configuration with defaults.
 */
void sd_wrapper_config_init(sd_wrapper_config_t* config) {
    if (config == NULL) {
        return;
    }

    memset(config, 0, sizeof(sd_wrapper_config_t));

    /* Defaults for SD 3.5 Medium on RTX 4070 Super (12GB VRAM) */
    config->model_path = NULL;
    config->clip_l_path = NULL;
    config->clip_g_path = NULL;
    config->t5xxl_path = NULL;
    config->vae_path = NULL;
    config->n_threads = -1;           /* Auto-detect */
    config->keep_clip_on_cpu = true;  /* Save VRAM */
    config->keep_vae_on_cpu = false;  /* VAE on GPU for speed */
    config->enable_flash_attn = true; /* Faster attention */
}

/**
 * Initialize generation parameters with defaults.
 */
void sd_wrapper_gen_params_init(sd_wrapper_gen_params_t* params) {
    if (params == NULL) {
        return;
    }

    memset(params, 0, sizeof(sd_wrapper_gen_params_t));

    /* Defaults for SD 3.5 Medium */
    params->prompt = NULL;
    params->negative_prompt = NULL;
    params->width = 1024;
    params->height = 1024;
    params->steps = 28;        /* SD 3.5 Medium default */
    params->cfg_scale = 4.5f;  /* SD 3.5 Medium default */
    params->seed = 0;          /* Random */
    params->clip_skip = 0;     /* No skip */
}

/**
 * Create a new SD wrapper context and load model.
 */
sd_wrapper_ctx_t* sd_wrapper_create(const sd_wrapper_config_t* config) {
    if (config == NULL || config->model_path == NULL) {
        return NULL;
    }

    /* Allocate context */
    sd_wrapper_ctx_t* ctx = calloc(1, sizeof(struct sd_wrapper_ctx));
    if (ctx == NULL) {
        return NULL;
    }

    /* Initialize context with default error message before model load */
    ctx->sd_ctx = NULL;
    snprintf(ctx->error_msg, sizeof(ctx->error_msg), "Model load failed");
    ctx->config = *config;
    ctx->callbacks_enabled = false;

    /* Set up logging callback */
    sd_set_log_callback(sd_wrapper_log_callback, ctx);

    /* Initialize stable-diffusion.cpp context parameters */
    sd_ctx_params_t sd_params;
    sd_ctx_params_init(&sd_params);

    /* Set model paths */
    sd_params.model_path = config->model_path;
    sd_params.clip_l_path = config->clip_l_path;
    sd_params.clip_g_path = config->clip_g_path;
    sd_params.t5xxl_path = config->t5xxl_path;
    sd_params.vae_path = config->vae_path;

    /* Set CPU/GPU offloading */
    sd_params.keep_clip_on_cpu = config->keep_clip_on_cpu;
    sd_params.keep_vae_on_cpu = config->keep_vae_on_cpu;

    /* Set threading */
    if (config->n_threads > 0) {
        sd_params.n_threads = config->n_threads;
    } else {
        sd_params.n_threads = sd_get_num_physical_cores();
    }

    /* Enable flash attention if requested */
    sd_params.diffusion_flash_attn = config->enable_flash_attn;

    /*
     * Use FP16 for model weights.
     * Trade-off: Saves ~50% VRAM with minimal quality loss for SD 3.5 Medium.
     * This is intentional for 12GB VRAM cards like RTX 4070 Super.
     */
    sd_params.wtype = SD_TYPE_F16;

    /* Enable Vulkan backend (will be set by build flags) */
    /* The library will automatically use Vulkan if built with -DSD_USE_VULKAN */

    /* Create stable-diffusion.cpp context */
    ctx->sd_ctx = new_sd_ctx(&sd_params);
    if (ctx->sd_ctx == NULL) {
        /* Model load failed - error_msg already set */
        free(ctx);
        return NULL;
    }

    /* Clear error message on success */
    ctx->error_msg[0] = '\0';

    return ctx;
}

/**
 * Free SD wrapper context and release resources.
 */
void sd_wrapper_free(sd_wrapper_ctx_t* ctx) {
    if (ctx == NULL) {
        return;
    }

    if (ctx->sd_ctx != NULL) {
        free_sd_ctx(ctx->sd_ctx);
        ctx->sd_ctx = NULL;
    }

    free(ctx);
}

/**
 * Generate an image from text prompt.
 */
sd_wrapper_error_t sd_wrapper_generate(sd_wrapper_ctx_t* ctx,
                                        const sd_wrapper_gen_params_t* params,
                                        sd_wrapper_image_t* image) {
    if (ctx == NULL || ctx->sd_ctx == NULL) {
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    if (params == NULL || params->prompt == NULL || image == NULL) {
        snprintf(ctx->error_msg, sizeof(ctx->error_msg),
                 "Invalid parameters: params, prompt, or image is NULL");
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* Validate dimensions */
    if (params->width < 64 || params->width > 2048 ||
        params->height < 64 || params->height > 2048 ||
        params->width % 64 != 0 || params->height % 64 != 0) {
        snprintf(ctx->error_msg, sizeof(ctx->error_msg),
                 "Invalid dimensions: must be 64-2048 and multiple of 64");
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* Validate steps */
    if (params->steps < 1 || params->steps > 100) {
        snprintf(ctx->error_msg, sizeof(ctx->error_msg),
                 "Invalid steps: must be 1-100");
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* Validate CFG scale */
    if (params->cfg_scale < 0.0f || params->cfg_scale > 20.0f) {
        snprintf(ctx->error_msg, sizeof(ctx->error_msg),
                 "Invalid CFG scale: must be 0.0-20.0");
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* Initialize generation parameters */
    sd_img_gen_params_t gen_params;
    sd_img_gen_params_init(&gen_params);

    /* Set prompts */
    gen_params.prompt = params->prompt;
    gen_params.negative_prompt = params->negative_prompt;

    /* Set dimensions */
    gen_params.width = params->width;
    gen_params.height = params->height;

    /* Set sampling parameters */
    gen_params.sample_params.sample_steps = params->steps;
    gen_params.sample_params.guidance.txt_cfg = params->cfg_scale;

    /* Get default sampler and scheduler from context */
    gen_params.sample_params.sample_method = sd_get_default_sample_method(ctx->sd_ctx);
    gen_params.sample_params.scheduler = sd_get_default_scheduler(
        ctx->sd_ctx,
        gen_params.sample_params.sample_method);

    /* Set seed */
    gen_params.seed = params->seed;

    /* Set CLIP skip */
    gen_params.clip_skip = params->clip_skip;

    /*
     * Set up progress and preview callbacks if enabled.
     *
     * These callbacks are global (not per-context) in stable-diffusion.cpp,
     * but this is safe because compute is single-threaded (one generation at a time).
     * Callbacks fire synchronously during generate_image() and send events to socket.
     */
    if (ctx->callbacks_enabled) {
        /* Set progress callback (fires every step) */
        sd_set_progress_callback(sd_wrapper_progress_callback, ctx);

        /* Set preview callback if preview_interval > 0 */
        if (ctx->callback_ctx.preview_interval > 0) {
            /*
             * PREVIEW_PROJ: Projection-based decoding (lowest quality, fastest).
             * No additional model files required.
             * denoised=true: Show denoised preview (cleaner).
             * noisy=false: Don't show noisy preview.
             */
            int interval = (ctx->callback_ctx.preview_interval <= (uint32_t)INT_MAX)
                ? (int)ctx->callback_ctx.preview_interval
                : INT_MAX;
            sd_set_preview_callback(sd_wrapper_preview_callback,
                                     PREVIEW_PROJ,
                                     interval,
                                     true,  /* denoised */
                                     false, /* noisy */
                                     ctx);
        }
    }

    /* Generate image */
    sd_image_t* sd_img = generate_image(ctx->sd_ctx, &gen_params);

    /*
     * Always clear callbacks after generation completes, even on failure.
     * Must happen before any early returns to prevent stale callback state.
     */
    if (ctx->callbacks_enabled) {
        sd_set_progress_callback(NULL, NULL);
        sd_set_preview_callback(NULL, PREVIEW_NONE, 0, false, false, NULL);
    }

    if (sd_img == NULL) {
        snprintf(ctx->error_msg, sizeof(ctx->error_msg),
                 "Image generation failed. Check GPU memory and model.");
        return SD_WRAPPER_ERR_GENERATION_FAILED;
    }

    /* Copy image data to output */
    image->width = sd_img->width;
    image->height = sd_img->height;
    image->channels = sd_img->channel;

    /* Check for integer overflow in size calculation */
    if (sd_img->width > SIZE_MAX / sd_img->height ||
        sd_img->width * sd_img->height > SIZE_MAX / sd_img->channel) {
        free(sd_img->data);
        free(sd_img);
        snprintf(ctx->error_msg, sizeof(ctx->error_msg),
                 "Image size calculation overflow");
        return SD_WRAPPER_ERR_OUT_OF_MEMORY;
    }

    image->data_size = sd_img->width * sd_img->height * sd_img->channel;

    /* Allocate buffer for caller */
    image->data = malloc(image->data_size);
    if (image->data == NULL) {
        free(sd_img->data);
        free(sd_img);
        snprintf(ctx->error_msg, sizeof(ctx->error_msg),
                 "Out of memory allocating image buffer");
        return SD_WRAPPER_ERR_OUT_OF_MEMORY;
    }

    /* Copy pixel data */
    memcpy(image->data, sd_img->data, image->data_size);

    /* Free the sd_img now that we've copied the data */
    free(sd_img->data);
    free(sd_img);

    return SD_WRAPPER_OK;
}

/**
 * Free image data allocated by sd_wrapper_generate().
 */
void sd_wrapper_free_image(sd_wrapper_image_t* image) {
    if (image == NULL) {
        return;
    }

    if (image->data != NULL) {
        free(image->data);
        image->data = NULL;
    }

    image->width = 0;
    image->height = 0;
    image->channels = 0;
    image->data_size = 0;
}

/**
 * Get error message for last error.
 */
const char* sd_wrapper_get_error(sd_wrapper_ctx_t* ctx) {
    if (ctx == NULL) {
        return "Invalid context";
    }

    return ctx->error_msg;
}

/**
 * Get model information.
 */
sd_wrapper_error_t sd_wrapper_get_model_info(sd_wrapper_ctx_t* ctx,
                                              char* model_name,
                                              size_t buf_size) {
    int ret;

    if (ctx == NULL || model_name == NULL || buf_size == 0) {
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* For now, just return the model path basename */
    const char* path = ctx->config.model_path;
    if (path == NULL) {
        ret = snprintf(model_name, buf_size, "unknown");
        if (ret < 0 || (size_t)ret >= buf_size) {
            /* Truncated or error - still return OK since we have partial result */
            model_name[buf_size - 1] = '\0';
        }
        return SD_WRAPPER_OK;
    }

    /* Find last slash */
    const char* basename = strrchr(path, '/');
    if (basename != NULL) {
        basename++; /* Skip the slash */
    } else {
        basename = path;
    }

    ret = snprintf(model_name, buf_size, "%s", basename);
    if (ret < 0 || (size_t)ret >= buf_size) {
        /* Truncated or error - still return OK since we have partial result */
        model_name[buf_size - 1] = '\0';
    }
    return SD_WRAPPER_OK;
}

/**
 * Reset the SD context to clean state.
 *
 * WORKAROUND: stable-diffusion.cpp has a bug where calling generate_image()
 * multiple times on the same context causes segfaults due to GGML compute
 * buffers not being properly freed between calls. This function destroys
 * and recreates the internal context to ensure clean state.
 */
sd_wrapper_error_t sd_wrapper_reset(sd_wrapper_ctx_t* ctx) {
    if (ctx == NULL) {
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* Free existing SD context if present */
    if (ctx->sd_ctx != NULL) {
        free_sd_ctx(ctx->sd_ctx);
        ctx->sd_ctx = NULL;
    }

    /* Recreate SD context with stored configuration */
    sd_ctx_params_t sd_params;
    sd_ctx_params_init(&sd_params);

    /* Set model paths from stored config */
    sd_params.model_path = ctx->config.model_path;
    sd_params.clip_l_path = ctx->config.clip_l_path;
    sd_params.clip_g_path = ctx->config.clip_g_path;
    sd_params.t5xxl_path = ctx->config.t5xxl_path;
    sd_params.vae_path = ctx->config.vae_path;

    /* Set CPU/GPU offloading */
    sd_params.keep_clip_on_cpu = ctx->config.keep_clip_on_cpu;
    sd_params.keep_vae_on_cpu = ctx->config.keep_vae_on_cpu;

    /* Set threading */
    if (ctx->config.n_threads > 0) {
        sd_params.n_threads = ctx->config.n_threads;
    } else {
        sd_params.n_threads = sd_get_num_physical_cores();
    }

    /* Enable flash attention if configured */
    sd_params.diffusion_flash_attn = ctx->config.enable_flash_attn;

    /* Use FP16 for model weights */
    sd_params.wtype = SD_TYPE_F16;

    /* Recreate context */
    ctx->sd_ctx = new_sd_ctx(&sd_params);
    if (ctx->sd_ctx == NULL) {
        snprintf(ctx->error_msg, sizeof(ctx->error_msg),
                 "Failed to recreate SD context");
        return SD_WRAPPER_ERR_INIT_FAILED;
    }

    ctx->error_msg[0] = '\0';
    return SD_WRAPPER_OK;
}

/**
 * Helper function to write full buffer to socket.
 *
 * Handles partial writes and EINTR. Does not handle timeouts.
 *
 * @param fd     Socket file descriptor
 * @param buf    Buffer to write
 * @param count  Number of bytes to write
 * @return       0 on success, -1 on error
 */
static int write_full(int fd, const uint8_t* buf, size_t count) {
    size_t total = 0;

    while (total < count) {
        ssize_t n = write(fd, buf + total, count - total);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        total += (size_t)n;
    }

    return 0;
}

/**
 * Set callback context for progress and preview events.
 */
sd_wrapper_error_t sd_wrapper_set_callback_ctx(sd_wrapper_ctx_t* ctx,
                                                 const sd_wrapper_callback_ctx_t* callback_ctx) {
    if (ctx == NULL) {
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    if (callback_ctx == NULL) {
        /* Disable callbacks */
        ctx->callbacks_enabled = false;
        memset(&ctx->callback_ctx, 0, sizeof(ctx->callback_ctx));
        return SD_WRAPPER_OK;
    }

    /* Enable callbacks with provided context */
    ctx->callback_ctx = *callback_ctx;
    ctx->callbacks_enabled = true;

    return SD_WRAPPER_OK;
}

/**
 * Progress callback for stable-diffusion.cpp.
 *
 * Called after each denoising step. Encodes and sends MSG_PROGRESS_EVENT
 * to the socket.
 *
 * @param step   Current step (1-based)
 * @param steps  Total steps
 * @param time   Step time in seconds
 * @param data   Callback context (sd_wrapper_ctx_t*)
 */
static void sd_wrapper_progress_callback(int step, int steps, float time, void* data) {
    sd_wrapper_ctx_t* ctx = (sd_wrapper_ctx_t*)data;

    if (ctx == NULL || !ctx->callbacks_enabled) {
        return;
    }

    if (ctx->callback_ctx.socket_fd < 0) {
        return;
    }

    /* Convert step time from seconds to milliseconds */
    uint32_t step_time_ms = (uint32_t)(time * 1000.0f);

    /* Build progress event */
    sd35_progress_event_t event;
    event.request_id = ctx->callback_ctx.request_id;
    event.step = (uint32_t)step;
    event.total_steps = (uint32_t)steps;
    event.step_time_ms = step_time_ms;

    /* Encode event to buffer */
    uint8_t buffer[256]; /* Progress events are small (~40 bytes) */
    size_t encoded_len;

    error_code_t err = encode_progress_event(&event, buffer, sizeof(buffer), &encoded_len);
    if (err != ERR_NONE) {
        fprintf(stderr, "[sd_wrapper] failed to encode progress event: %d\n", err);
        return;
    }

    /* Write to socket */
    if (write_full(ctx->callback_ctx.socket_fd, buffer, encoded_len) != 0) {
        fprintf(stderr, "[sd_wrapper] failed to write progress event: %s\n", strerror(errno));
        return;
    }
}

/**
 * Preview callback for stable-diffusion.cpp.
 *
 * Called every preview_interval steps with a decoded preview image.
 * Encodes and sends MSG_PREVIEW_EVENT to the socket.
 *
 * @param step        Current step (1-based)
 * @param frame_count Number of preview frames (should be 1)
 * @param frames      Preview image array
 * @param is_noisy    Whether preview is noisy (unused)
 * @param data        Callback context (sd_wrapper_ctx_t*)
 */
static void sd_wrapper_preview_callback(int step, int frame_count,
                                         sd_image_t* frames, bool is_noisy, void* data) {
    (void)is_noisy; /* Unused */

    sd_wrapper_ctx_t* ctx = (sd_wrapper_ctx_t*)data;

    if (ctx == NULL || !ctx->callbacks_enabled) {
        return;
    }

    if (ctx->callback_ctx.socket_fd < 0) {
        return;
    }

    if (frames == NULL || frame_count < 1) {
        fprintf(stderr, "[sd_wrapper] preview callback called with no frames\n");
        return;
    }

    /* Use first frame (stable-diffusion.cpp typically sends 1 frame) */
    sd_image_t* frame = &frames[0];

    if (frame->data == NULL) {
        fprintf(stderr, "[sd_wrapper] preview frame has no data\n");
        return;
    }

    /* Calculate image data size */
    size_t image_data_len = (size_t)frame->width * frame->height * frame->channel;

    /* Validate size fits in uint32_t (protocol constraint) */
    if (image_data_len > UINT32_MAX) {
        fprintf(stderr, "[sd_wrapper] preview image too large: %zu bytes\n", image_data_len);
        return;
    }

    /* Build preview event */
    sd35_preview_event_t event;
    event.request_id = ctx->callback_ctx.request_id;
    event.step = (uint32_t)step;
    event.total_steps = ctx->callback_ctx.total_steps;
    event.width = frame->width;
    event.height = frame->height;
    event.channels = frame->channel;
    event.image_data_len = (uint32_t)image_data_len;
    event.image_data = frame->data;

    /*
     * Allocate buffer for encoded event.
     * Size = header (16) + metadata (32) + image data.
     * Preview images can be large (e.g., 1024x1024 RGB = 3MB).
     */
    size_t buffer_size = 16 + 32 + image_data_len;
    uint8_t* buffer = malloc(buffer_size);
    if (buffer == NULL) {
        fprintf(stderr, "[sd_wrapper] failed to allocate preview buffer (%zu bytes)\n", buffer_size);
        return;
    }

    size_t encoded_len;
    error_code_t err = encode_preview_event(&event, buffer, buffer_size, &encoded_len);
    if (err != ERR_NONE) {
        fprintf(stderr, "[sd_wrapper] failed to encode preview event: %d\n", err);
        free(buffer);
        return;
    }

    /* Write to socket */
    if (write_full(ctx->callback_ctx.socket_fd, buffer, encoded_len) != 0) {
        fprintf(stderr, "[sd_wrapper] failed to write preview event: %s\n", strerror(errno));
        free(buffer);
        return;
    }

    free(buffer);
}

/**
 * Logging callback for stable-diffusion.cpp.
 */
static void sd_wrapper_log_callback(enum sd_log_level_t level,
                                     const char* text,
                                     void* data) {
    (void)data; /* Unused for now */

    /* Map SD log levels to stderr output */
    const char* level_str = "INFO";
    switch (level) {
        case SD_LOG_DEBUG:
            level_str = "DEBUG";
            break;
        case SD_LOG_INFO:
            level_str = "INFO";
            break;
        case SD_LOG_WARN:
            level_str = "WARN";
            break;
        case SD_LOG_ERROR:
            level_str = "ERROR";
            break;
    }

    fprintf(stderr, "[sd] %s: %s\n", level_str, text);
}