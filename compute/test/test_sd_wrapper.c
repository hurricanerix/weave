/**
 * Basic test for SD wrapper integration.
 *
 * This test verifies:
 * - Wrapper functions exist and link correctly
 * - Configuration initialization works
 * - Parameter initialization works
 * - Context creation fails gracefully with invalid model path
 *
 * Note: This does NOT test actual model loading or generation,
 * as that requires a real model file and GPU.
 */

#include "weave/sd_wrapper.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>

void test_config_init(void) {
    sd_wrapper_config_t config;
    sd_wrapper_config_init(&config);

    assert(config.model_path == NULL);
    assert(config.n_threads == -1);
    assert(config.keep_clip_on_cpu == true);
    assert(config.keep_vae_on_cpu == false);
    assert(config.enable_flash_attn == true);

    printf("[test_config_init] PASS\n");
}

void test_gen_params_init(void) {
    sd_wrapper_gen_params_t params;
    sd_wrapper_gen_params_init(&params);

    assert(params.prompt == NULL);
    assert(params.negative_prompt == NULL);
    assert(params.width == 1024);
    assert(params.height == 1024);
    assert(params.steps == 28);
    assert(params.cfg_scale == 4.5f);
    assert(params.seed == 0);
    assert(params.clip_skip == 0);

    printf("[test_gen_params_init] PASS\n");
}

void test_create_null_config(void) {
    sd_wrapper_ctx_t* ctx = sd_wrapper_create(NULL);
    assert(ctx == NULL);

    printf("[test_create_null_config] PASS\n");
}

void test_create_null_model_path(void) {
    sd_wrapper_config_t config;
    sd_wrapper_config_init(&config);

    /* model_path is NULL */
    sd_wrapper_ctx_t* ctx = sd_wrapper_create(&config);
    assert(ctx == NULL);

    printf("[test_create_null_model_path] PASS\n");
}

void test_create_invalid_model_path(void) {
    sd_wrapper_config_t config;
    sd_wrapper_config_init(&config);

    /* Non-existent model file */
    config.model_path = "/nonexistent/model.safetensors";

    sd_wrapper_ctx_t* ctx = sd_wrapper_create(&config);

    /* Context should be created, but sd_ctx will be NULL */
    /* This is expected behavior - context creation defers model loading */
    if (ctx != NULL) {
        const char* error = sd_wrapper_get_error(ctx);
        printf("[test_create_invalid_model_path] Context created, error: %s\n", error);
        sd_wrapper_free(ctx);
    }

    printf("[test_create_invalid_model_path] PASS\n");
}

void test_free_null_context(void) {
    /* Should not crash */
    sd_wrapper_free(NULL);

    printf("[test_free_null_context] PASS\n");
}

void test_free_image_null(void) {
    /* Should not crash */
    sd_wrapper_free_image(NULL);

    printf("[test_free_image_null] PASS\n");
}

void test_get_error_null_context(void) {
    const char* error = sd_wrapper_get_error(NULL);
    assert(error != NULL);
    assert(strcmp(error, "Invalid context") == 0);

    printf("[test_get_error_null_context] PASS\n");
}

int main(void) {
    printf("Running SD wrapper tests...\n");

    test_config_init();
    test_gen_params_init();
    test_create_null_config();
    test_create_null_model_path();
    test_create_invalid_model_path();
    test_free_null_context();
    test_free_image_null();
    test_get_error_null_context();

    printf("\nAll SD wrapper tests passed.\n");
    printf("\nNote: These tests verify API correctness only.\n");
    printf("Model loading and generation require a real model file and GPU.\n");
    printf("See docs/DEVELOPMENT.md for model download instructions.\n");

    return 0;
}
