/**
 * Wrapper implementation for stable-diffusion.cpp integration.
 *
 * This file bridges our C99 interface to stable-diffusion.cpp's C++ API.
 */

#include "weave/sd_wrapper.h"

#include <cstdlib>
#include <cstring>
#include <new>      /* For std::nothrow */
#include <string>

/* Include stable-diffusion.cpp C API */
#include "stable-diffusion.h"

/**
 * Internal context structure.
 * Holds stable-diffusion.cpp context and error state.
 */
struct sd_wrapper_ctx {
    sd_ctx_t* sd_ctx;           /* stable-diffusion.cpp context */
    std::string error_msg;      /* Last error message */
    sd_wrapper_config_t config; /* Configuration used to create context */
};

/* Forward declarations */
static void sd_wrapper_log_callback(enum sd_log_level_t level,
                                     const char* text,
                                     void* data);

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

    /* Allocate context using nothrow to return NULL instead of throwing */
    sd_wrapper_ctx_t* ctx = new(std::nothrow) sd_wrapper_ctx;
    if (ctx == NULL) {
        return NULL;
    }

    /* Initialize context with default error message before model load */
    ctx->sd_ctx = NULL;
    ctx->error_msg = "Model load failed";  /* Default error before attempting load */
    ctx->config = *config;

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
        delete ctx;
        return NULL;
    }

    /* Clear error message on success */
    ctx->error_msg = "";

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

    delete ctx;
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
        ctx->error_msg = "Invalid parameters: params, prompt, or image is NULL";
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* Validate dimensions */
    if (params->width < 64 || params->width > 2048 ||
        params->height < 64 || params->height > 2048 ||
        params->width % 64 != 0 || params->height % 64 != 0) {
        ctx->error_msg = "Invalid dimensions: must be 64-2048 and multiple of 64";
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* Validate steps */
    if (params->steps < 1 || params->steps > 100) {
        ctx->error_msg = "Invalid steps: must be 1-100";
        return SD_WRAPPER_ERR_INVALID_PARAM;
    }

    /* Validate CFG scale */
    if (params->cfg_scale < 0.0f || params->cfg_scale > 20.0f) {
        ctx->error_msg = "Invalid CFG scale: must be 0.0-20.0";
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

    /* Generate image */
    sd_image_t* sd_img = generate_image(ctx->sd_ctx, &gen_params);
    if (sd_img == NULL) {
        ctx->error_msg = "Image generation failed. Check GPU memory and model.";
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
        ctx->error_msg = "Image size calculation overflow";
        return SD_WRAPPER_ERR_OUT_OF_MEMORY;
    }

    image->data_size = sd_img->width * sd_img->height * sd_img->channel;

    /* Allocate buffer for caller */
    image->data = (uint8_t*)malloc(image->data_size);
    if (image->data == NULL) {
        free(sd_img->data);
        free(sd_img);
        ctx->error_msg = "Out of memory allocating image buffer";
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

    return ctx->error_msg.c_str();
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
