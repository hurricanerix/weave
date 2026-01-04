/**
 * Weave Compute Daemon - Request Processing Pipeline
 *
 * This module bridges protocol requests and the SD wrapper, handling:
 * - Parameter conversion from protocol to SD wrapper format
 * - Image generation orchestration
 * - Error mapping and response building
 *
 * Ownership model:
 * - Input: Caller owns request structure (borrowed)
 * - Output: Response structure owns image data (caller must free)
 * - SD context: Passed through, not owned by this module
 */

#pragma once

#include <stdint.h>
#include "weave/protocol.h"
#include "weave/sd_wrapper.h"

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
error_code_t process_generate_request(sd_wrapper_ctx_t *ctx,
                                       const sd35_generate_request_t *req,
                                       sd35_generate_response_t *resp);

/**
 * Free response image data allocated by process_generate_request().
 *
 * This is a convenience wrapper around sd_wrapper_free_image().
 * Safe to call with NULL or already-freed image data.
 *
 * @param resp  Response structure (image_data will be freed and set to NULL)
 */
void free_generate_response(sd35_generate_response_t *resp);
