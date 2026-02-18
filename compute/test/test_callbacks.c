/**
 * Test for callback setup during generation.
 *
 * This test verifies that:
 * - Callbacks are enabled when preview_interval > 0
 * - Callbacks are disabled when preview_interval == 0
 * - Callback context contains correct metadata
 *
 * Note: This is a unit test using mocks. Full integration testing with
 * actual callbacks firing requires a real model and GPU.
 */

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "weave/generate.h"
#include "weave/protocol.h"
#include "weave/sd_wrapper.h"

/**
 * Mock SD wrapper to verify callback setup
 */

typedef struct {
    sd_wrapper_callback_ctx_t last_callback_ctx;
    bool callbacks_were_enabled;
    bool callbacks_were_disabled;
    int set_callback_call_count;
} mock_sd_ctx_t;

static mock_sd_ctx_t mock_ctx;

void sd_wrapper_gen_params_init(sd_wrapper_gen_params_t* params) {
    memset(params, 0, sizeof(*params));
}

sd_wrapper_error_t sd_wrapper_set_callback_ctx(sd_wrapper_ctx_t* ctx,
                                                 const sd_wrapper_callback_ctx_t* callback_ctx) {
    mock_sd_ctx_t* mock = (mock_sd_ctx_t*)ctx;
    mock->set_callback_call_count++;

    if (callback_ctx == NULL) {
        mock->callbacks_were_disabled = true;
    } else {
        mock->callbacks_were_enabled = true;
        mock->last_callback_ctx = *callback_ctx;
    }

    return SD_WRAPPER_OK;
}

sd_wrapper_error_t sd_wrapper_generate(sd_wrapper_ctx_t* ctx,
                                        const sd_wrapper_gen_params_t* params,
                                        sd_wrapper_image_t* image) {
    (void)ctx;

    /* Return success with small test image */
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
    (void)ctx;
    return SD_WRAPPER_OK;
}

/**
 * Helper to create a valid test request
 */
static sd35_generate_request_t create_test_request(uint32_t preview_interval) {
    sd35_generate_request_t req;
    memset(&req, 0, sizeof(req));

    req.request_id = 12345;
    req.model_id = MODEL_ID_SD35;
    req.width = 512;
    req.height = 512;
    req.steps = 10;
    req.cfg_scale = 7.0f;
    req.seed = 42;
    req.preview_interval = preview_interval;

    static const char prompt_text[] = "test prompt";
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
 * Test: Callbacks are enabled when preview_interval > 0
 */
void test_callbacks_enabled_with_preview_interval(void) {
    memset(&mock_ctx, 0, sizeof(mock_ctx));

    sd35_generate_request_t req = create_test_request(5);
    sd35_generate_response_t resp;
    memset(&resp, 0, sizeof(resp));

    int socket_fd = 42; /* Dummy fd (not actually used by mock) */
    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx,
                                                  socket_fd, &req, &resp);

    assert(err == ERR_NONE);
    assert(mock_ctx.set_callback_call_count == 1);
    assert(mock_ctx.callbacks_were_enabled);
    assert(mock_ctx.last_callback_ctx.socket_fd == socket_fd);
    assert(mock_ctx.last_callback_ctx.request_id == req.request_id);
    assert(mock_ctx.last_callback_ctx.preview_interval == req.preview_interval);
    assert(mock_ctx.last_callback_ctx.total_steps == req.steps);

    free_generate_response(&resp);

    printf("PASS: test_callbacks_enabled_with_preview_interval\n");
}

/**
 * Test: Callbacks are disabled when preview_interval == 0
 */
void test_callbacks_disabled_when_preview_interval_zero(void) {
    memset(&mock_ctx, 0, sizeof(mock_ctx));

    sd35_generate_request_t req = create_test_request(0);
    sd35_generate_response_t resp;
    memset(&resp, 0, sizeof(resp));

    int socket_fd = 42;
    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx,
                                                  socket_fd, &req, &resp);

    assert(err == ERR_NONE);
    assert(mock_ctx.set_callback_call_count == 1);
    assert(mock_ctx.callbacks_were_disabled);
    assert(!mock_ctx.callbacks_were_enabled);

    free_generate_response(&resp);

    printf("PASS: test_callbacks_disabled_when_preview_interval_zero\n");
}

/**
 * Test: Callback context contains correct metadata
 */
void test_callback_context_metadata(void) {
    memset(&mock_ctx, 0, sizeof(mock_ctx));

    sd35_generate_request_t req = create_test_request(3);
    req.request_id = 99999;
    req.steps = 28;

    sd35_generate_response_t resp;
    memset(&resp, 0, sizeof(resp));

    int socket_fd = 123;
    error_code_t err = process_generate_request((sd_wrapper_ctx_t*)&mock_ctx,
                                                  socket_fd, &req, &resp);

    assert(err == ERR_NONE);
    assert(mock_ctx.callbacks_were_enabled);
    assert(mock_ctx.last_callback_ctx.socket_fd == 123);
    assert(mock_ctx.last_callback_ctx.request_id == 99999);
    assert(mock_ctx.last_callback_ctx.preview_interval == 3);
    assert(mock_ctx.last_callback_ctx.total_steps == 28);

    free_generate_response(&resp);

    printf("PASS: test_callback_context_metadata\n");
}

int main(void) {
    printf("Running callback setup tests...\n\n");

    test_callbacks_enabled_with_preview_interval();
    test_callbacks_disabled_when_preview_interval_zero();
    test_callback_context_metadata();

    printf("\nAll callback setup tests passed!\n");
    printf("\nNote: These tests verify callback setup logic only.\n");
    printf("Full integration testing with actual progress/preview events\n");
    printf("requires a real model file and GPU. See docs/DEVELOPMENT.md\n");
    printf("for instructions on running integration tests.\n");

    return 0;
}
