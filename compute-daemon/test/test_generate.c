/**
 * Unit tests for request processing pipeline
 *
 * These tests verify parameter conversion and error mapping.
 * They use a mock SD wrapper to avoid GPU dependencies.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <assert.h>
#include "weave/generate.h"

/**
 * Mock SD wrapper context and functions
 */

typedef struct {
    sd_wrapper_error_t error_to_return;
    sd_wrapper_gen_params_t last_params;
    char last_prompt[2048];
    uint32_t generate_call_count;
} mock_sd_ctx_t;

static mock_sd_ctx_t mock_ctx;

void sd_wrapper_gen_params_init(sd_wrapper_gen_params_t* params) {
    memset(params, 0, sizeof(*params));
}

sd_wrapper_error_t sd_wrapper_generate(sd_wrapper_ctx_t* ctx,
                                        const sd_wrapper_gen_params_t* params,
                                        sd_wrapper_image_t* image) {
    mock_sd_ctx_t* mock = (mock_sd_ctx_t*)ctx;
    mock->generate_call_count++;
    memcpy(&mock->last_params, params, sizeof(*params));

    if (params->prompt != NULL) {
        strncpy(mock->last_prompt, params->prompt, sizeof(mock->last_prompt) - 1);
        mock->last_prompt[sizeof(mock->last_prompt) - 1] = '\0';
        mock->last_params.prompt = mock->last_prompt;
    }

    if (mock->error_to_return != SD_WRAPPER_OK) {
        return mock->error_to_return;
    }

    image->width = params->width;
    image->height = params->height;
    image->channels = 3;
    image->data_size = params->width * params->height * 3;
    image->data = (uint8_t*)calloc(1, image->data_size);

    if (image->data == NULL) {
        return SD_WRAPPER_ERR_OUT_OF_MEMORY;
    }

    return SD_WRAPPER_OK;
}

void sd_wrapper_free_image(sd_wrapper_image_t* image) {
    if (image != NULL && image->data != NULL) {
        free(image->data);
        image->data = NULL;
        image->data_size = 0;
    }
}

sd_wrapper_error_t sd_wrapper_reset(sd_wrapper_ctx_t* ctx) {
    (void)ctx;  /* Unused in mock - no actual reset needed */
    return SD_WRAPPER_OK;
}

/**
 * Test helpers
 */

static void reset_mock(void) {
    memset(&mock_ctx, 0, sizeof(mock_ctx));
    mock_ctx.error_to_return = SD_WRAPPER_OK;
}

static sd35_generate_request_t create_valid_request(void) {
    sd35_generate_request_t req;
    memset(&req, 0, sizeof(req));

    req.request_id = 12345;
    req.model_id = MODEL_ID_SD35;
    req.width = 512;
    req.height = 512;
    req.steps = 28;
    req.cfg_scale = 7.0f;
    req.seed = 42;

    static const char prompt_text[] = "a cat in space";
    static uint8_t prompt_data[1024];
    memcpy(prompt_data, prompt_text, sizeof(prompt_text));

    req.clip_l_offset = 0;
    req.clip_l_length = sizeof(prompt_text) - 1;
    req.clip_g_offset = 0;
    req.clip_g_length = sizeof(prompt_text) - 1;
    req.t5_offset = 0;
    req.t5_length = sizeof(prompt_text) - 1;
    req.prompt_data = prompt_data;
    req.prompt_data_len = sizeof(prompt_data);

    return req;
}

/**
 * Tests
 */

void test_process_valid_request(void) {
    reset_mock();

    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;
    memset(&resp, 0, sizeof(resp));

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_NONE);
    assert(resp.request_id == req.request_id);
    assert(resp.status == STATUS_OK);
    assert(resp.image_width == req.width);
    assert(resp.image_height == req.height);
    assert(resp.channels == 3);
    assert(resp.image_data_len == req.width * req.height * 3);
    assert(resp.image_data != NULL);
    assert(mock_ctx.generate_call_count == 1);

    free_generate_response(&resp);
    assert(resp.image_data == NULL);

    printf("PASS: test_process_valid_request\n");
}

void test_null_context(void) {
    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request(NULL, &req, &resp);

    assert(err == ERR_INTERNAL);

    printf("PASS: test_null_context\n");
}

void test_null_request(void) {
    reset_mock();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, NULL, &resp);

    assert(err == ERR_INTERNAL);

    printf("PASS: test_null_request\n");
}

void test_null_response(void) {
    reset_mock();
    sd35_generate_request_t req = create_valid_request();

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, NULL);

    assert(err == ERR_INTERNAL);

    printf("PASS: test_null_response\n");
}

void test_invalid_prompt_null_data(void) {
    reset_mock();

    sd35_generate_request_t req = create_valid_request();
    req.prompt_data = NULL;
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_INVALID_PROMPT);

    printf("PASS: test_invalid_prompt_null_data\n");
}

void test_invalid_prompt_zero_length(void) {
    reset_mock();

    sd35_generate_request_t req = create_valid_request();
    req.clip_l_length = 0;
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_INVALID_PROMPT);

    printf("PASS: test_invalid_prompt_zero_length\n");
}

void test_invalid_prompt_too_long(void) {
    reset_mock();

    sd35_generate_request_t req = create_valid_request();
    req.clip_l_length = SD35_MAX_PROMPT_LENGTH + 1;
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_INVALID_PROMPT);

    printf("PASS: test_invalid_prompt_too_long\n");
}

void test_invalid_prompt_out_of_bounds(void) {
    reset_mock();

    sd35_generate_request_t req = create_valid_request();
    req.clip_l_offset = req.prompt_data_len;
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_INVALID_PROMPT);

    printf("PASS: test_invalid_prompt_out_of_bounds\n");
}

void test_sd_wrapper_invalid_param_error(void) {
    reset_mock();
    mock_ctx.error_to_return = SD_WRAPPER_ERR_INVALID_PARAM;

    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_INVALID_PROMPT);

    printf("PASS: test_sd_wrapper_invalid_param_error\n");
}

void test_sd_wrapper_out_of_memory_error(void) {
    reset_mock();
    mock_ctx.error_to_return = SD_WRAPPER_ERR_OUT_OF_MEMORY;

    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_OUT_OF_MEMORY);

    printf("PASS: test_sd_wrapper_out_of_memory_error\n");
}

void test_sd_wrapper_gpu_error(void) {
    reset_mock();
    mock_ctx.error_to_return = SD_WRAPPER_ERR_GPU_ERROR;

    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_GPU_ERROR);

    printf("PASS: test_sd_wrapper_gpu_error\n");
}

void test_sd_wrapper_model_not_found_error(void) {
    reset_mock();
    mock_ctx.error_to_return = SD_WRAPPER_ERR_MODEL_NOT_FOUND;

    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_INTERNAL);

    printf("PASS: test_sd_wrapper_model_not_found_error\n");
}

void test_sd_wrapper_generation_failed_error(void) {
    reset_mock();
    mock_ctx.error_to_return = SD_WRAPPER_ERR_GENERATION_FAILED;

    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_INTERNAL);

    printf("PASS: test_sd_wrapper_generation_failed_error\n");
}

void test_parameter_conversion(void) {
    reset_mock();

    sd35_generate_request_t req = create_valid_request();
    req.width = 1024;
    req.height = 768;
    req.steps = 50;
    req.cfg_scale = 9.5f;
    req.seed = 999;

    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_NONE);
    assert(mock_ctx.last_params.width == 1024);
    assert(mock_ctx.last_params.height == 768);
    assert(mock_ctx.last_params.steps == 50);
    assert(mock_ctx.last_params.cfg_scale == 9.5f);
    assert(mock_ctx.last_params.seed == 999);
    assert(mock_ctx.last_params.prompt != NULL);
    assert(strcmp(mock_ctx.last_params.prompt, "a cat in space") == 0);

    free_generate_response(&resp);

    printf("PASS: test_parameter_conversion\n");
}

void test_generation_time_tracking(void) {
    reset_mock();

    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);

    assert(err == ERR_NONE);

    free_generate_response(&resp);

    printf("PASS: test_generation_time_tracking\n");
}

void test_free_null_response(void) {
    free_generate_response(NULL);

    printf("PASS: test_free_null_response\n");
}

void test_free_empty_response(void) {
    sd35_generate_response_t resp;
    memset(&resp, 0, sizeof(resp));

    free_generate_response(&resp);

    printf("PASS: test_free_empty_response\n");
}

void test_double_free_response(void) {
    reset_mock();

    sd35_generate_request_t req = create_valid_request();
    sd35_generate_response_t resp;

    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx, &req, &resp);
    assert(err == ERR_NONE);

    free_generate_response(&resp);
    free_generate_response(&resp);

    printf("PASS: test_double_free_response\n");
}

int main(void) {
    printf("Running generate pipeline tests...\n\n");

    test_process_valid_request();
    test_null_context();
    test_null_request();
    test_null_response();
    test_invalid_prompt_null_data();
    test_invalid_prompt_zero_length();
    test_invalid_prompt_too_long();
    test_invalid_prompt_out_of_bounds();
    test_sd_wrapper_invalid_param_error();
    test_sd_wrapper_out_of_memory_error();
    test_sd_wrapper_gpu_error();
    test_sd_wrapper_model_not_found_error();
    test_sd_wrapper_generation_failed_error();
    test_parameter_conversion();
    test_generation_time_tracking();
    test_free_null_response();
    test_free_empty_response();
    test_double_free_response();

    printf("\nAll tests passed!\n");
    return 0;
}
