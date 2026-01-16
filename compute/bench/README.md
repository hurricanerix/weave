# Weave Compute Benchmarks

Performance benchmarks for Stable Diffusion generation.

## Building

```bash
make bench
```

This creates `bench/bench_generate` binary.

## Requirements

1. SD 3.5 Medium model file (see `docs/DEVELOPMENT.md` for download)
2. GPU with Vulkan support (NVIDIA or AMD)
3. Sufficient VRAM (12GB recommended)

## Running

Basic usage:

```bash
./bench/bench_generate <model_path> [iterations]
```

Example:

```bash
./bench/bench_generate models/sd3.5_medium.safetensors 10
```

## Benchmark Configurations

The benchmark runs three configurations:

1. Fast Baseline (512x512, 4 steps)
   - Quick generation test
   - Verifies model loads correctly

2. Target Config (1024x1024, 4 steps)
   - Primary performance target
   - Must complete in under 3 seconds on RTX 4070 Super

3. Quality Config (1024x1024, 8 steps)
   - Higher quality generation
   - No specific time target

## Output Format

```
=== Weave Compute Benchmark ===

Model: SD 3.5 Medium
GPU: NVIDIA GeForce RTX 4070 SUPER
Iterations: 10

Loading model...
Model loaded successfully.

VRAM Usage: ~10.2GB

Running: Fast Baseline (512x512, 4 steps)
  Running 10 iterations...
  [progress output]

Running: Target Config (1024x1024, 4 steps)
  Running 10 iterations...
  [progress output]

Running: Quality Config (1024x1024, 8 steps)
  Running 10 iterations...
  [progress output]

=== Results ===

Configuration: Fast Baseline (512x512, 4 steps)
  Resolution: 512x512
  Steps: 4
  CFG Scale: 4.5

  Timing:
    Min:     780.23 ms (0.78 s)
    Max:     850.12 ms (0.85 s)
    Avg:     810.45 ms (0.81 s)
    Median:  808.67 ms (0.81 s)

Configuration: Target Config (1024x1024, 4 steps)
  Resolution: 1024x1024
  Steps: 4
  CFG Scale: 4.5

  Timing:
    Min:    2100.34 ms (2.10 s)
    Max:    2250.67 ms (2.25 s)
    Avg:    2180.23 ms (2.18 s)
    Median: 2175.45 ms (2.18 s)

  Target: < 3000.00 ms (3.00 s)
  Status: PASS

Configuration: Quality Config (1024x1024, 8 steps)
  Resolution: 1024x1024
  Steps: 8
  CFG Scale: 4.5

  Timing:
    Min:    4200.56 ms (4.20 s)
    Max:    4350.89 ms (4.35 s)
    Avg:    4280.12 ms (4.28 s)
    Median: 4275.34 ms (4.28 s)

=== Overall Status ===

Target performance (1024x1024, 4 steps < 3s): PASS

Benchmark complete.
```

## Measuring VRAM Usage

The benchmark attempts to detect VRAM usage automatically using `nvidia-smi`.

Manual VRAM check during benchmark:

```bash
# In another terminal while benchmark runs
nvidia-smi

# Or watch continuously
watch -n 1 nvidia-smi
```

For AMD GPUs:

```bash
rocm-smi
```

## Performance Targets

Target hardware: RTX 4070 Super (12GB VRAM)

| Configuration | Resolution | Steps | Target Time |
|--------------|------------|-------|-------------|
| Fast Baseline | 512x512 | 4 | No target |
| Target Config | 1024x1024 | 4 | < 3.0s |
| Quality Config | 1024x1024 | 8 | No target |

## Troubleshooting

If benchmark fails or performance is poor:

1. Check GPU is not under load: `nvidia-smi`
2. Check thermal throttling: `nvidia-smi -q -d TEMPERATURE`
3. Verify Vulkan is installed: `vulkaninfo`
4. Check model file exists and is correct version
5. Ensure flash attention is enabled (default)

Performance tips:

- Close other GPU applications
- Use `keep_clip_on_cpu=true` to save VRAM
- Set `enable_flash_attn=true` for speed
- Keep GPU cool (check fans, airflow)

## Exit Codes

- 0: All benchmarks passed, target met
- 1: Benchmark failed or target not met
