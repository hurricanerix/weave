/**
 * Performance benchmark for Stable Diffusion generation.
 *
 * This benchmark:
 * - Loads SD 3.5 Medium model once at startup
 * - Runs multiple generation iterations
 * - Measures timing for each iteration
 * - Reports min/max/avg/median performance
 * - Validates against target performance (1024x1024, 4 steps < 3s)
 *
 * Usage:
 *   bench_generate <model_path> [iterations]
 *
 * Example:
 *   bench_generate models/sd3.5_medium.safetensors 10
 *
 * Target hardware: RTX 4070 Super (12GB VRAM)
 * Target performance: 1024x1024, 4 steps in under 3 seconds
 */

#define _POSIX_C_SOURCE 199309L /* For clock_gettime */

#include "weave/sd_wrapper.h"

#include <assert.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

/* Benchmark configuration */
typedef struct {
    const char* model_path;
    int iterations;
    bool verbose;
} bench_config_t;

/* Benchmark result for one configuration */
typedef struct {
    const char* name;
    uint32_t width;
    uint32_t height;
    uint32_t steps;
    float cfg_scale;
    double min_ms;
    double max_ms;
    double avg_ms;
    double median_ms;
    bool target_pass; /* Did this meet target? */
    double target_ms;
} bench_result_t;

/* Timing measurement */
static double get_time_ms(struct timespec* start, struct timespec* end) {
    double start_ms = start->tv_sec * 1000.0 + start->tv_nsec / 1000000.0;
    double end_ms = end->tv_sec * 1000.0 + end->tv_nsec / 1000000.0;
    return end_ms - start_ms;
}

/* Comparison function for qsort (for median calculation) */
static int compare_double(const void* a, const void* b) {
    double diff = *(const double*)a - *(const double*)b;
    if (diff < 0.0) return -1;
    if (diff > 0.0) return 1;
    return 0;
}

/* Calculate statistics from timing array */
static void calculate_stats(double* times, int count, bench_result_t* result) {
    if (count == 0) {
        result->min_ms = 0.0;
        result->max_ms = 0.0;
        result->avg_ms = 0.0;
        result->median_ms = 0.0;
        return;
    }

    /* Min, max, sum */
    double min = times[0];
    double max = times[0];
    double sum = 0.0;

    for (int i = 0; i < count; i++) {
        if (times[i] < min) min = times[i];
        if (times[i] > max) max = times[i];
        sum += times[i];
    }

    result->min_ms = min;
    result->max_ms = max;
    result->avg_ms = sum / (double)count;

    /* Median - sort array and take middle */
    qsort(times, count, sizeof(double), compare_double);

    if (count % 2 == 0) {
        result->median_ms = (times[count / 2 - 1] + times[count / 2]) / 2.0;
    } else {
        result->median_ms = times[count / 2];
    }
}

/* Run benchmark for one configuration */
static bool run_benchmark(sd_wrapper_ctx_t* ctx,
                         bench_config_t* bench_config,
                         bench_result_t* result) {
    double* times = (double*)malloc(sizeof(double) * bench_config->iterations);
    if (times == NULL) {
        fprintf(stderr, "Error: Failed to allocate timing array\n");
        return false;
    }

    sd_wrapper_gen_params_t params;
    sd_wrapper_gen_params_init(&params);

    params.prompt = "a cat in space, digital art";
    params.negative_prompt = NULL;
    params.width = result->width;
    params.height = result->height;
    params.steps = result->steps;
    params.cfg_scale = result->cfg_scale;
    params.seed = 42; /* Fixed seed for reproducibility */

    if (bench_config->verbose) {
        printf("  Running %d iterations...\n", bench_config->iterations);
    }

    /* Run iterations */
    for (int i = 0; i < bench_config->iterations; i++) {
        sd_wrapper_image_t image;
        struct timespec start, end;

        /* Measure generation time */
        clock_gettime(CLOCK_MONOTONIC, &start);
        sd_wrapper_error_t err = sd_wrapper_generate(ctx, &params, &image);
        clock_gettime(CLOCK_MONOTONIC, &end);

        if (err != SD_WRAPPER_OK) {
            fprintf(stderr, "Error: Generation failed on iteration %d: %s\n",
                    i + 1, sd_wrapper_get_error(ctx));
            free(times);
            return false;
        }

        times[i] = get_time_ms(&start, &end);

        if (bench_config->verbose) {
            printf("    Iteration %d/%d: %.2f ms\n",
                   i + 1, bench_config->iterations, times[i]);
        }

        /* Free image data */
        sd_wrapper_free_image(&image);
    }

    /* Calculate statistics */
    calculate_stats(times, bench_config->iterations, result);

    /* Check if target was met */
    if (result->target_ms > 0.0) {
        result->target_pass = (result->avg_ms <= result->target_ms);
    } else {
        result->target_pass = true; /* No target defined */
    }

    free(times);
    return true;
}

/* Print benchmark results */
static void print_results(bench_result_t* results, int count) {
    printf("\n");
    printf("=== Results ===\n");
    printf("\n");

    for (int i = 0; i < count; i++) {
        bench_result_t* r = &results[i];

        printf("Configuration: %s\n", r->name);
        printf("  Resolution: %ux%u\n", r->width, r->height);
        printf("  Steps: %u\n", r->steps);
        printf("  CFG Scale: %.1f\n", r->cfg_scale);
        printf("\n");
        printf("  Timing:\n");
        printf("    Min:    %7.2f ms (%.2f s)\n", r->min_ms, r->min_ms / 1000.0);
        printf("    Max:    %7.2f ms (%.2f s)\n", r->max_ms, r->max_ms / 1000.0);
        printf("    Avg:    %7.2f ms (%.2f s)\n", r->avg_ms, r->avg_ms / 1000.0);
        printf("    Median: %7.2f ms (%.2f s)\n", r->median_ms, r->median_ms / 1000.0);

        if (r->target_ms > 0.0) {
            printf("\n");
            printf("  Target: < %.2f ms (%.2f s)\n", r->target_ms, r->target_ms / 1000.0);
            printf("  Status: %s\n", r->target_pass ? "PASS" : "FAIL");
        }

        printf("\n");
    }
}

/* Detect GPU name using nvidia-smi or rocm-smi */
static void detect_gpu(char* gpu_name, size_t buf_size) {
    FILE* fp = popen("nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null", "r");
    if (fp != NULL) {
        if (fgets(gpu_name, buf_size, fp) != NULL) {
            /* Remove trailing newline */
            size_t len = strlen(gpu_name);
            if (len > 0 && gpu_name[len - 1] == '\n') {
                gpu_name[len - 1] = '\0';
            }
            pclose(fp);
            return;
        }
        pclose(fp);
    }

    /* Try ROCm */
    fp = popen("rocm-smi --showproductname 2>/dev/null | grep 'Card series' | awk '{print $3}'", "r");
    if (fp != NULL) {
        if (fgets(gpu_name, buf_size, fp) != NULL) {
            size_t len = strlen(gpu_name);
            if (len > 0 && gpu_name[len - 1] == '\n') {
                gpu_name[len - 1] = '\0';
            }
            pclose(fp);
            return;
        }
        pclose(fp);
    }

    /* Fallback */
    snprintf(gpu_name, buf_size, "Unknown GPU");
}

/* Get VRAM usage (NVIDIA only for now) */
static void get_vram_usage(char* vram_str, size_t buf_size) {
    FILE* fp = popen("nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits 2>/dev/null", "r");
    if (fp != NULL) {
        char buffer[64];
        if (fgets(buffer, sizeof(buffer), fp) != NULL) {
            unsigned long vram_mb = strtoul(buffer, NULL, 10);
            double vram_gb = (double)vram_mb / 1024.0;
            snprintf(vram_str, buf_size, "~%.1f GB", vram_gb);
            pclose(fp);
            return;
        }
        pclose(fp);
    }

    /* Fallback */
    snprintf(vram_str, buf_size, "Unknown (run nvidia-smi manually)");
}

int main(int argc, char** argv) {
    if (argc < 2) {
        fprintf(stderr, "Usage: %s <model_path> [iterations]\n", argv[0]);
        fprintf(stderr, "\n");
        fprintf(stderr, "Example:\n");
        fprintf(stderr, "  %s models/sd3.5_medium.safetensors 10\n", argv[0]);
        fprintf(stderr, "\n");
        fprintf(stderr, "Note: Model file must exist and GPU must be available.\n");
        return 1;
    }

    bench_config_t bench_config = {
        .model_path = argv[1],
        .iterations = 10,
        .verbose = true,
    };

    if (argc >= 3) {
        char *endptr;
        long val = strtol(argv[2], &endptr, 10);

        /* Check for conversion errors */
        if (endptr == argv[2] || *endptr != '\0') {
            fprintf(stderr, "Error: Invalid iterations value: '%s'\n", argv[2]);
            return 1;
        }

        if (val < 1 || val > 1000) {
            fprintf(stderr, "Error: Iterations must be between 1 and 1000\n");
            return 1;
        }

        bench_config.iterations = (int)val;
    }

    printf("=== Weave Compute Benchmark ===\n");
    printf("\n");
    printf("Model: SD 3.5 Medium\n");

    /* Detect GPU */
    char gpu_name[256];
    detect_gpu(gpu_name, sizeof(gpu_name));
    printf("GPU: %s\n", gpu_name);

    printf("Iterations: %d\n", bench_config.iterations);
    printf("\n");

    /* Load model */
    printf("Loading model...\n");

    /* Construct text encoder paths from model directory */
    char clip_l_path[512];
    char clip_g_path[512];
    char t5_path[512];

    /* Extract directory from model path */
    const char* model_path = bench_config.model_path;
    const char* last_slash = strrchr(model_path, '/');
    size_t dir_len = last_slash ? (size_t)(last_slash - model_path + 1) : 0;

    if (dir_len > 0) {
        snprintf(clip_l_path, sizeof(clip_l_path), "%.*sclip_l.safetensors", (int)dir_len, model_path);
        snprintf(clip_g_path, sizeof(clip_g_path), "%.*sclip_g.safetensors", (int)dir_len, model_path);
        snprintf(t5_path, sizeof(t5_path), "%.*st5xxl_fp8_e4m3fn.safetensors", (int)dir_len, model_path);
    } else {
        snprintf(clip_l_path, sizeof(clip_l_path), "clip_l.safetensors");
        snprintf(clip_g_path, sizeof(clip_g_path), "clip_g.safetensors");
        snprintf(t5_path, sizeof(t5_path), "t5xxl_fp8_e4m3fn.safetensors");
    }

    sd_wrapper_config_t config;
    sd_wrapper_config_init(&config);
    config.model_path = bench_config.model_path;
    config.clip_l_path = clip_l_path;
    config.clip_g_path = clip_g_path;
    config.t5xxl_path = t5_path;
    config.keep_clip_on_cpu = true;  /* Save VRAM */
    config.keep_vae_on_cpu = false;
    config.enable_flash_attn = true;

    printf("  Main model: %s\n", config.model_path);
    printf("  CLIP-L: %s\n", config.clip_l_path);
    printf("  CLIP-G: %s\n", config.clip_g_path);
    printf("  T5-XXL: %s\n", config.t5xxl_path);
    printf("\n");

    sd_wrapper_ctx_t* ctx = sd_wrapper_create(&config);
    if (ctx == NULL) {
        fprintf(stderr, "Error: Failed to create SD context\n");
        fprintf(stderr, "Make sure model file exists: %s\n", bench_config.model_path);
        return 1;
    }

    /* Check if model loaded successfully */
    const char* error = sd_wrapper_get_error(ctx);
    if (error != NULL && strlen(error) > 0) {
        fprintf(stderr, "Error: Model loading failed: %s\n", error);
        sd_wrapper_free(ctx);
        return 1;
    }

    printf("Model loaded successfully.\n");
    printf("\n");

    /* Get VRAM usage after model load */
    char vram_str[64];
    get_vram_usage(vram_str, sizeof(vram_str));
    printf("VRAM Usage: %s\n", vram_str);
    printf("\n");

    /* Define benchmark configurations */
    bench_result_t results[3] = {
        {
            .name = "Fast Baseline (512x512, 4 steps)",
            .width = 512,
            .height = 512,
            .steps = 4,
            .cfg_scale = 4.5f,
            .min_ms = 0.0,
            .max_ms = 0.0,
            .avg_ms = 0.0,
            .median_ms = 0.0,
            .target_pass = false,
            .target_ms = 0.0, /* No specific target */
        },
        {
            .name = "Target Config (1024x1024, 4 steps)",
            .width = 1024,
            .height = 1024,
            .steps = 4,
            .cfg_scale = 4.5f,
            .min_ms = 0.0,
            .max_ms = 0.0,
            .avg_ms = 0.0,
            .median_ms = 0.0,
            .target_pass = false,
            .target_ms = 3000.0, /* 3 second target */
        },
        {
            .name = "Quality Config (1024x1024, 8 steps)",
            .width = 1024,
            .height = 1024,
            .steps = 8,
            .cfg_scale = 4.5f,
            .min_ms = 0.0,
            .max_ms = 0.0,
            .avg_ms = 0.0,
            .median_ms = 0.0,
            .target_pass = false,
            .target_ms = 0.0, /* No specific target */
        },
    };

    /* Run benchmarks */
    for (int i = 0; i < 3; i++) {
        printf("Running: %s\n", results[i].name);

        if (!run_benchmark(ctx, &bench_config, &results[i])) {
            fprintf(stderr, "Error: Benchmark failed for configuration: %s\n",
                    results[i].name);
            sd_wrapper_free(ctx);
            return 1;
        }
    }

    /* Print results */
    print_results(results, 3);

    /* Overall status */
    printf("=== Overall Status ===\n");
    printf("\n");

    bool target_met = results[1].target_pass; /* 1024x1024, 4 steps */
    printf("Target performance (1024x1024, 4 steps < 3s): %s\n",
           target_met ? "PASS" : "FAIL");

    if (!target_met) {
        printf("\n");
        printf("Performance did not meet target. Possible reasons:\n");
        printf("  - Different GPU (target is RTX 4070 Super)\n");
        printf("  - GPU under load from other processes\n");
        printf("  - Model not optimized (check flash attention settings)\n");
        printf("  - Thermal throttling\n");
        printf("\n");
        printf("Run 'nvidia-smi' to check GPU utilization and temperature.\n");
    }

    /* Cleanup */
    sd_wrapper_free(ctx);

    printf("\nBenchmark complete.\n");

    return target_met ? 0 : 1;
}
