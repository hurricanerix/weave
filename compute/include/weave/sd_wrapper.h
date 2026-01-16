#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

/**
 * Thin wrapper around stable-diffusion.cpp for weave-compute.
 *
 * This wrapper provides a simplified interface for SD 3.5 Medium inference
 * with Vulkan backend. It handles model loading, text encoding, and image
 * generation.
 */

#ifdef __cplusplus
extern "C" {
#endif

/**
 * Error codes returned by wrapper functions.
 */
typedef enum {
    SD_WRAPPER_OK = 0,
    SD_WRAPPER_ERR_INVALID_PARAM = -1,
    SD_WRAPPER_ERR_MODEL_NOT_FOUND = -2,
    SD_WRAPPER_ERR_MODEL_CORRUPT = -3,
    SD_WRAPPER_ERR_OUT_OF_MEMORY = -4,
    SD_WRAPPER_ERR_GPU_ERROR = -5,
    SD_WRAPPER_ERR_INIT_FAILED = -6,
    SD_WRAPPER_ERR_GENERATION_FAILED = -7,
} sd_wrapper_error_t;

/**
 * Opaque context for SD wrapper.
 * Holds stable-diffusion.cpp context and configuration.
 */
typedef struct sd_wrapper_ctx sd_wrapper_ctx_t;

/**
 * Configuration for SD wrapper initialization.
 */
typedef struct {
    const char* model_path;           /* Path to main model file (.safetensors or .gguf) */
    const char* clip_l_path;          /* Path to CLIP-L encoder (NULL for auto-detect) */
    const char* clip_g_path;          /* Path to CLIP-G encoder (NULL for auto-detect) */
    const char* t5xxl_path;           /* Path to T5-XXL encoder (NULL for auto-detect) */
    const char* vae_path;             /* Path to VAE (NULL for auto-detect) */
    int n_threads;                    /* Number of CPU threads (-1 for auto) */
    bool keep_clip_on_cpu;            /* Keep text encoders on CPU (saves VRAM) */
    bool keep_vae_on_cpu;             /* Keep VAE on CPU (saves VRAM) */
    bool enable_flash_attn;           /* Enable flash attention (faster) */
} sd_wrapper_config_t;

/**
 * Parameters for image generation.
 */
typedef struct {
    const char* prompt;               /* Text prompt (required) */
    const char* negative_prompt;      /* Negative prompt (NULL for none) */
    uint32_t width;                   /* Image width (64-2048, multiple of 64) */
    uint32_t height;                  /* Image height (64-2048, multiple of 64) */
    uint32_t steps;                   /* Sampling steps (1-100) */
    float cfg_scale;                  /* Guidance scale (0.0-20.0) */
    int64_t seed;                     /* Random seed (0 for random) */
    int clip_skip;                    /* CLIP skip layers (0 for default) */
} sd_wrapper_gen_params_t;

/**
 * Generated image data.
 */
typedef struct {
    uint32_t width;                   /* Image width in pixels */
    uint32_t height;                  /* Image height in pixels */
    uint32_t channels;                /* Number of channels (3=RGB, 4=RGBA) */
    uint8_t* data;                    /* Raw pixel data (caller must free) */
    size_t data_size;                 /* Size of data buffer in bytes */
} sd_wrapper_image_t;

/**
 * Initialize wrapper configuration with defaults.
 *
 * @param config  Pointer to config structure to initialize
 */
void sd_wrapper_config_init(sd_wrapper_config_t* config);

/**
 * Initialize generation parameters with defaults.
 *
 * @param params  Pointer to params structure to initialize
 */
void sd_wrapper_gen_params_init(sd_wrapper_gen_params_t* params);

/**
 * Create a new SD wrapper context and load model.
 *
 * This function:
 * - Loads the SD 3.5 Medium model from disk
 * - Initializes Vulkan backend
 * - Loads text encoders (CLIP-L, CLIP-G, T5-XXL)
 * - Allocates GPU memory for model weights
 *
 * @param config  Configuration for model loading
 * @return        Context on success, NULL on failure
 *
 * @note Model remains loaded until sd_wrapper_free() is called
 * @note This function may take several seconds to complete
 */
sd_wrapper_ctx_t* sd_wrapper_create(const sd_wrapper_config_t* config);

/**
 * Free SD wrapper context and release resources.
 *
 * @param ctx  Context to free (NULL safe)
 */
void sd_wrapper_free(sd_wrapper_ctx_t* ctx);

/**
 * Generate an image from text prompt.
 *
 * This function:
 * - Encodes text prompt using CLIP and T5 encoders
 * - Runs diffusion model on GPU
 * - Decodes latents to RGB image
 *
 * @param ctx     SD wrapper context (must not be NULL)
 * @param params  Generation parameters
 * @param image   Output image (caller must free image->data)
 * @return        SD_WRAPPER_OK on success, error code on failure
 *
 * @note image->data must be freed by caller using free()
 * @note Generation time depends on steps and resolution (1-10s typical)
 */
sd_wrapper_error_t sd_wrapper_generate(sd_wrapper_ctx_t* ctx,
                                        const sd_wrapper_gen_params_t* params,
                                        sd_wrapper_image_t* image);

/**
 * Free image data allocated by sd_wrapper_generate().
 *
 * @param image  Image to free (NULL safe)
 */
void sd_wrapper_free_image(sd_wrapper_image_t* image);

/**
 * Get error message for last error.
 *
 * @param ctx  SD wrapper context
 * @return     Error message string (valid until next wrapper call)
 */
const char* sd_wrapper_get_error(sd_wrapper_ctx_t* ctx);

/**
 * Get model information.
 *
 * @param ctx         SD wrapper context
 * @param model_name  Output buffer for model name
 * @param buf_size    Size of output buffer
 * @return            SD_WRAPPER_OK on success, error code on failure
 */
sd_wrapper_error_t sd_wrapper_get_model_info(sd_wrapper_ctx_t* ctx,
                                              char* model_name,
                                              size_t buf_size);

/**
 * Reset the SD context to clean state.
 *
 * This function destroys and recreates the internal stable-diffusion.cpp
 * context to ensure clean state between generations.
 *
 * WORKAROUND: This is needed because stable-diffusion.cpp has a bug where
 * GGML compute buffers are not properly freed between generate_image() calls,
 * causing segfaults on subsequent generations with different prompt lengths.
 *
 * @param ctx  SD wrapper context (must not be NULL)
 * @return     SD_WRAPPER_OK on success, error code on failure
 *
 * @note This operation takes 2-3 seconds as the model must be reloaded.
 * @note Call this before each generation to avoid segfaults.
 */
sd_wrapper_error_t sd_wrapper_reset(sd_wrapper_ctx_t* ctx);

#ifdef __cplusplus
}
#endif
