# Bug 003: GGML assertion failure with long prompts

## Status

Fixed

## Summary

The compute daemon crashes with a GGML assertion failure when processing prompts where the T5 encoder produces more tokens than the CLIP encoders. This occurs because stable-diffusion.cpp has a bug in compute buffer sizing when token counts mismatch across encoders.

## Symptoms

**User-facing error:**
```
Connection to image generation service was closed
```

**Log output (compute daemon):**
```
[sd] DEBUG: clip.hpp:304  - token length: 154
[sd] DEBUG: clip.hpp:304  - token length: 154
[sd] DEBUG: t5.hpp:402  - token length: 231
[sd] DEBUG: ggml_extend.hpp:1754 - clip compute buffer size: 1.40 MB(RAM)
...
/home/.../stable-diffusion.cpp/ggml/src/ggml-cpu/ops.cpp:4666: GGML_ASSERT(i01 >= 0 && i01 < ne01) failed
[1]    183712 IOT instruction (core dumped)  ./compute/weave-compute
```

## Steps to reproduce

1. Start weave-compute daemon
2. Start weave web server
3. Generate an image with a short prompt (~550 chars) - works
4. Generate an image with a longer prompt (~800+ chars) where T5 produces more tokens than CLIP
5. Daemon crashes with GGML assertion failure

## Expected behavior

Prompts of any length should either be processed successfully or rejected with a clear error message before reaching the GPU.

## Actual behavior

The daemon crashes with an assertion failure in GGML's CPU operations, terminating the process and closing all client connections.

## Root cause analysis

The crash occurs when T5 produces more tokens than CLIP for the same prompt. CLIP and T5 use different tokenizers:
- CLIP tokenizers (CLIP-L, CLIP-G) have internal limits and may truncate
- T5 tokenizer handles longer sequences without truncation

When T5 token count exceeds CLIP token count, the compute buffer sizing in stable-diffusion.cpp becomes incorrect, causing an index out of bounds access.

**Token count observations:**
- 550 chars → 154 CLIP, 154 T5 (match) → OK
- 800 chars → 154 CLIP, 231 T5 (mismatch) → CRASH
- 1500 chars → 308 CLIP, 385 T5 (mismatch) → CRASH

The pattern is clear: crashes occur when T5 tokens > CLIP tokens.

## Environment

- GPU: NVIDIA GeForce RTX 4070 SUPER
- Backend: Vulkan
- Model: SD3.5 Medium (sd3.5_medium.safetensors)

## Resolution

Reduced maximum prompt length from 2048 to 512 bytes per encoder.

**Rationale:** 512 bytes produces approximately 128-170 tokens. This keeps prompts short enough that CLIP and T5 produce similar token counts, avoiding the mismatch that triggers the crash.

**Files changed:**
- `compute/include/weave/protocol.h` - `SD35_MAX_PROMPT_LENGTH` 2048 → 512
- `internal/protocol/types.go` - `SD35MaxPromptLen` 2048 → 512
- `internal/protocol/types_test.go` - Updated test expectations
- `internal/protocol/encode_test.go` - Updated test expectations

**Trade-off:** Users are now limited to ~512 character prompts. The LLM agent may generate longer prompts which will be rejected. Consider adding prompt truncation on the Go side if this becomes problematic.

## Related

- Bug 002: Segmentation fault on second image generation (different root cause, same symptom)

## Priority

High - Crashes the daemon when prompts exceed safe length.
