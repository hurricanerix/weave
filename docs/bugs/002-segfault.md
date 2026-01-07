# Bug 002: Segmentation fault on second image generation

## Status

Fixed

## Resolution

stable-diffusion.cpp has a bug where GGML compute buffers are not properly freed between `generate_image()` calls on the same context. This causes segfaults on subsequent generations.

**Fix:** Reset SD context before each generation.
- Added `sd_wrapper_reset()` function that destroys and recreates the context
- Modified `process_generate_request()` to call reset before each generation

**Trade-off:** Adds ~2-3 seconds model reload time per generation. This should be removed once the upstream stable-diffusion.cpp bug is fixed.

**Files changed:**
- `compute-daemon/include/weave/sd_wrapper.h` - Added `sd_wrapper_reset()` declaration
- `compute-daemon/src/sd_wrapper.cpp` - Implemented `sd_wrapper_reset()`
- `compute-daemon/src/generate.c` - Call reset before each generation
- `compute-daemon/test/test_generate.c` - Added mock for testing

**Verification:** All unit tests pass. Rebuild the daemon with `make -C compute-daemon` and restart to apply the fix

## Related

- Bug 003: GGML assertion failure with long prompts (different root cause, same symptom)

## Summary

The compute daemon (weave-compute) crashes with a segmentation fault when a user attempts to generate a second image. The first image generates successfully, but the second generation causes the daemon to crash, resulting in the user seeing "Connection to image generation service was closed" in the chat window.

## Symptoms

**User-facing error:**
```
Connection to image generation service was closed
```

**Log output (compute daemon):**
```
[sd] DEBUG: clip.hpp:304  - token length: 231
[sd] DEBUG: clip.hpp:304  - token length: 231
[sd] DEBUG: t5.hpp:402  - token length: 308
[sd] DEBUG: ggml_extend.hpp:1754 - clip compute buffer size: 1.40 MB(RAM)
[1]    143345 segmentation fault (core dumped)  ./compute-daemon/weave-compute
```

## Steps to reproduce

1. Start weave-compute daemon
2. Start weave web server
3. Generate an image (completes successfully)
4. Generate a second image (daemon crashes with segfault)

## Expected behavior

Multiple images can be generated in sequence without the daemon crashing.

## Actual behavior

The daemon crashes on the second image generation attempt. Users must restart the daemon to generate another image.

## Root cause analysis

The segfault occurs in the compute daemon during the second image generation, specifically during CLIP tokenization or shortly after. Possible causes:

- Memory not being properly released between generation requests
- Buffer reuse without proper reinitialization
- State corruption from the first generation affecting the second
- Use-after-free in the stable-diffusion.cpp or related code
- Double-free when cleaning up resources between requests

**Early warning sign in logs:**
```
failed to read request header
[socket] WARN: handler returned error: -1
```
This appears before the first successful generation and may indicate socket handling issues.

## Environment

- GPU: NVIDIA GeForce RTX 4070 SUPER
- Backend: Vulkan
- Model: SD3.5 Medium (sd3.5_medium.safetensors)
- VRAM usage: ~5.3GB for diffusion model + VAE

## Technical details

**First generation (successful):**
- Prompt tokenized (154 tokens)
- Sampling completed in 1.46s
- VAE decode completed in 1.28s
- Total time: 4.94s

**Second generation (crash):**
- Prompt tokenized (231/308 tokens - longer prompt)
- Crashes during or immediately after CLIP compute buffer allocation
- Segfault occurs before sampling begins

## Affected components

- `weave-compute` (C) - Core compute daemon
- Likely in: stable-diffusion.cpp, clip.hpp, or ggml_extend.hpp

## Investigation steps

1. Run under Valgrind to detect memory errors:
   ```bash
   valgrind --leak-check=full ./compute-daemon/weave-compute
   ```

2. Run under AddressSanitizer:
   ```bash
   make clean && make CFLAGS="-fsanitize=address -g" weave-compute
   ```

3. Generate core dump and analyze:
   ```bash
   ulimit -c unlimited
   ./compute-daemon/weave-compute
   # After crash:
   gdb ./compute-daemon/weave-compute core
   ```

4. Check if issue is related to prompt length (second prompt was longer)

5. Check if compute buffers are properly reset between requests

## Priority

Critical - This prevents users from generating more than one image per daemon session, severely impacting usability.

## Workaround

Restart the compute daemon between image generations.
