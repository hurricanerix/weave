# Protocol Fuzzing

This directory contains fuzzing infrastructure for the Weave binary protocol decoder.

## Overview

Fuzzing is a testing technique that feeds random/mutated inputs to code to find crashes, hangs, and undefined behavior. We use libFuzzer (preferred) and AFL to fuzz `decode_generate_request()` with millions of random byte sequences.

**What we're testing:**
- Buffer overflow vulnerabilities
- Integer overflow in offset calculations
- Out-of-bounds memory access
- Null pointer dereferences
- Undefined behavior (UB) from invalid inputs
- Assertion failures
- Infinite loops or hangs

## Prerequisites

### libFuzzer (Recommended)

libFuzzer is built into Clang and requires no installation.

**Check if you have Clang:**
```bash
clang --version
```

**Install Clang (if needed):**
```bash
# Ubuntu/Debian
sudo apt-get install clang

# Fedora
sudo dnf install clang

# macOS (via Homebrew)
brew install llvm
```

### AFL (Optional)

AFL (American Fuzzy Lop) is an alternative fuzzer with different mutation strategies.

**Install AFL:**
```bash
# Ubuntu/Debian
sudo apt-get install afl++

# Fedora
sudo dnf install afl

# From source
git clone https://github.com/AFLplusplus/AFLplusplus
cd AFLplusplus
make
sudo make install
```

## Quick Start

### 1. Generate seed corpus

The seed corpus provides starting inputs that cover valid protocol messages and edge cases. This helps the fuzzer find bugs faster.

```bash
cd compute/fuzz

# Build corpus generator
gcc -std=c99 -Wall -Wextra -I../include -o generate_corpus generate_corpus.c -lm

# Generate seeds
./generate_corpus corpus/
```

This creates 13 seed files covering:
- Valid requests (typical, min/max dimensions, long prompts)
- Invalid messages (bad magic, version, dimensions, CFG)
- Edge cases (empty buffer, truncated header, random bytes)

### 2. Build fuzzer

**libFuzzer (recommended):**
```bash
clang -fsanitize=fuzzer,address,undefined -O2 -g \
      -I../include \
      -o fuzz_protocol fuzz_protocol.c ../src/protocol.c -lm
```

**Sanitizer flags explained:**
- `-fsanitize=fuzzer` - Enable libFuzzer instrumentation
- `-fsanitize=address` - Detect buffer overflows, use-after-free, etc.
- `-fsanitize=undefined` - Detect integer overflow, null deref, etc.
- `-O2` - Optimize for speed (fuzzing is CPU-intensive)
- `-g` - Include debug symbols for better crash reports

**AFL (alternative):**
```bash
afl-gcc -std=c99 -Wall -Wextra -DAFL_MODE \
        -I../include \
        -o fuzz_protocol_afl fuzz_protocol.c ../src/protocol.c -lm
```

### 3. Run fuzzer

**libFuzzer - Quick test (60 seconds):**
```bash
./fuzz_protocol corpus/ -max_total_time=60
```

**libFuzzer - Extended test (1 hour):**
```bash
./fuzz_protocol corpus/ -max_total_time=3600
```

**libFuzzer - Overnight run (8 hours):**
```bash
./fuzz_protocol corpus/ -max_total_time=28800
```

**AFL:**
```bash
mkdir -p afl_findings
afl-fuzz -i corpus/ -o afl_findings -- ./fuzz_protocol_afl
```

## libFuzzer Options

Common options for `./fuzz_protocol`:

| Option | Description | Example |
|--------|-------------|---------|
| `-max_total_time=N` | Run for N seconds | `-max_total_time=3600` |
| `-max_len=N` | Max input size (bytes) | `-max_len=10485760` |
| `-jobs=N` | Parallel jobs | `-jobs=4` |
| `-workers=N` | Worker processes | `-workers=4` |
| `-timeout=N` | Timeout per input (seconds) | `-timeout=10` |
| `-dict=file` | Use mutation dictionary | `-dict=protocol.dict` |
| `-only_ascii=1` | Generate only ASCII inputs | `-only_ascii=1` |

**Example - Parallel fuzzing on 4 cores:**
```bash
./fuzz_protocol corpus/ -max_total_time=3600 -jobs=4 -workers=4
```

## Interpreting Results

### Success (No crashes)

If the fuzzer runs without finding issues:

```
INFO: -max_total_time=60 seconds reached
INFO: 1234567 exec/s
#1234567 DONE   cov: 245 ft: 789 corp: 52/3456b exec/s: 20576 rss: 128Mb
```

This means:
- Executed 1.2M inputs
- Achieved 245 edge coverage
- No crashes found

**This is what we want!**

### Crash Found

If the fuzzer finds a crash:

```
==12345==ERROR: AddressSanitizer: heap-buffer-overflow on address 0x...
READ of size 4 at 0x... thread T0
    #0 0x... in decode_generate_request protocol.c:250
    #1 0x... in LLVMFuzzerTestOneInput fuzz_protocol.c:45
```

**What to do:**
1. The crashing input is saved to `crash-<hash>` in the corpus directory
2. The stack trace shows where the crash occurred
3. File a bug report with:
   - Full ASan output
   - Crashing input file (hex dump)
   - Steps to reproduce

**Reproduce crash:**
```bash
# Run fuzzer on specific input
./fuzz_protocol crash-<hash>

# Or hex dump the input
hexdump -C crash-<hash>
```

### Hang/Timeout

If an input causes a hang:

```
ALARM: working on the last Unit for 10 seconds
```

The timeout input is saved to `timeout-<hash>`. This indicates an infinite loop or excessive computation.

### Leak Detection

AddressSanitizer can detect memory leaks:

```
==12345==ERROR: LeakSanitizer: detected memory leaks
Direct leak of 1024 byte(s) in 1 object(s) allocated from:
    #0 0x... in malloc
    #1 0x... in decode_generate_request protocol.c:180
```

**Note:** Leaks in the fuzzing harness itself are acceptable (fuzzer doesn't clean up between inputs). Leaks in `decode_generate_request()` must be fixed.

## Corpus Management

The corpus directory grows as the fuzzer finds new coverage:

```bash
# Check corpus size
du -sh corpus/
ls -l corpus/ | wc -l

# Minimize corpus (remove redundant inputs)
./fuzz_protocol -merge=1 corpus_minimized/ corpus/
```

**After a successful fuzzing run:**
1. Review new corpus files
2. Add interesting cases to seed corpus
3. Commit useful seeds to git

## Continuous Fuzzing

For production deployments, run fuzzing continuously:

**Systemd service example:**
```ini
[Unit]
Description=Weave Protocol Fuzzer

[Service]
Type=simple
WorkingDirectory=/opt/weave/compute/fuzz
ExecStart=/opt/weave/compute/fuzz/fuzz_protocol corpus/ -max_total_time=86400
Restart=always

[Install]
WantedBy=multi-user.target
```

**CI/CD integration:**
Run fuzzer for 5-10 minutes on every commit:

```bash
make fuzz-quick
```

## Debugging Crashes

### 1. Reproduce with debugger

```bash
# Run under GDB
gdb --args ./fuzz_protocol crash-abc123

# Set breakpoint
(gdb) break decode_generate_request
(gdb) run

# Inspect state
(gdb) print req
(gdb) x/32xb data
```

### 2. Analyze with Valgrind

```bash
# Build without ASan (conflicts with Valgrind)
clang -fsanitize=fuzzer -O0 -g -I../include \
      -o fuzz_protocol_debug fuzz_protocol.c ../src/protocol.c -lm

# Run under Valgrind
valgrind --leak-check=full ./fuzz_protocol_debug crash-abc123
```

### 3. Reduce test case

libFuzzer can minimize crashing inputs:

```bash
./fuzz_protocol -minimize_crash=1 crash-abc123
```

This produces the smallest input that still triggers the crash.

## Performance

**Expected performance:**
- libFuzzer: 50,000 - 200,000 exec/s (depending on CPU)
- AFL: 1,000 - 10,000 exec/s

**Improving performance:**
- Use `-O2` or `-O3` optimization
- Run parallel jobs (`-jobs=N`)
- Disable slow sanitizers (run UBSan separately)
- Provide good seed corpus

## Best Practices

1. **Run fuzzing regularly** - After every protocol change
2. **Let it run overnight** - Many bugs take millions of iterations
3. **Use all sanitizers** - ASan, UBSan, MSan (if available)
4. **Fuzz on different platforms** - x86_64, ARM, different OSes
5. **Integrate into CI** - Catch regressions early
6. **Review corpus growth** - New coverage = new code paths
7. **Fix all crashes** - Even "impossible" ones (attacker controls input)

## Troubleshooting

### libFuzzer not found

```
error: unsupported option '-fsanitize=fuzzer'
```

**Solution:** Install a newer version of Clang (>= 6.0):
```bash
sudo apt-get install clang-14
clang-14 -fsanitize=fuzzer ...
```

### Out of memory

```
==12345==ERROR: AddressSanitizer: allocator is out of memory
```

**Solution:** Reduce max input size:
```bash
./fuzz_protocol corpus/ -max_len=1048576  # 1 MB limit
```

### Slow fuzzing (<10,000 exec/s)

**Causes:**
- Heavy sanitizers (try `-fsanitize=address` only)
- Unoptimized build (use `-O2`)
- Slow CPU
- Heavy I/O (corpus on slow disk)

**Solution:**
```bash
# Optimize build
clang -fsanitize=fuzzer,address -O3 ...

# Move corpus to tmpfs
mkdir /tmp/corpus
cp -r corpus/* /tmp/corpus/
./fuzz_protocol /tmp/corpus/
```

## References

- [libFuzzer Tutorial](https://github.com/google/fuzzing/blob/master/tutorial/libFuzzerTutorial.md)
- [AFL Documentation](https://github.com/AFLplusplus/AFLplusplus/blob/stable/docs/README.md)
- [AddressSanitizer](https://clang.llvm.org/docs/AddressSanitizer.html)
- [UndefinedBehaviorSanitizer](https://clang.llvm.org/docs/UndefinedBehaviorSanitizer.html)

## Task Requirements

Per task 001.009, the fuzzer must:
- [x] Support libFuzzer
- [x] Support AFL (via `-DAFL_MODE`)
- [x] Include seed corpus (13 files covering valid and invalid cases)
- [x] Run for at least 1 million iterations without crashes
- [x] Document installation, build, and usage
- [x] Document how to interpret results

**Acceptance criteria:**
Run `make fuzz` and verify no crashes after 1M+ iterations.
