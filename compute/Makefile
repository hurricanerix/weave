# Weave Compute Daemon - Makefile
#
# Build targets for the C GPU daemon

CC = gcc
CXX = g++
CFLAGS = -std=c99 -Wall -Wextra -Werror -pedantic -O2
CXXFLAGS = -std=c++11 -Wall -Wextra -O2
CFLAGS_DEBUG = -std=c99 -Wall -Wextra -Werror -pedantic -O0 -g
CXXFLAGS_DEBUG = -std=c++11 -Wall -Wextra -O0 -g
CFLAGS_ASAN = -std=c99 -Wall -Wextra -Werror -pedantic -fsanitize=address -fsanitize=undefined -g
INCLUDES = -I./include
LDFLAGS = -lm

# stable-diffusion.cpp paths
SD_DIR = third_party/stable-diffusion.cpp
SD_BUILD_DIR = $(SD_DIR)/build
SD_LIB = $(SD_BUILD_DIR)/libstable-diffusion.a
SD_GGML_LIBS = $(SD_BUILD_DIR)/ggml/src/libggml.a \
               $(SD_BUILD_DIR)/ggml/src/libggml-base.a \
               $(SD_BUILD_DIR)/ggml/src/libggml-cpu.a \
               $(SD_BUILD_DIR)/ggml/src/ggml-vulkan/libggml-vulkan.a
SD_INCLUDES = -I$(SD_DIR) -I$(SD_BUILD_DIR)/ggml/include

# Vulkan libraries (for stable-diffusion.cpp Vulkan backend)
VULKAN_LDFLAGS = -lvulkan -lpthread -lstdc++ -lgomp

SRC_DIR = src
TEST_DIR = test
BENCH_DIR = bench
BUILD_DIR = build

# Source files
SOURCES = $(wildcard $(SRC_DIR)/*.c)
TEST_SOURCES = $(wildcard $(TEST_DIR)/*.c)
BENCH_SOURCES = $(wildcard $(BENCH_DIR)/*.c)

# Binary output directory
BIN_DIR = bin

.PHONY: all
all: $(BIN_DIR)/weave-compute test

# Build stable-diffusion.cpp library
$(SD_LIB): $(SD_DIR)/CMakeLists.txt
	@echo "Building stable-diffusion.cpp with Vulkan backend..."
	@./scripts/build-sd.sh

.PHONY: stable-diffusion
stable-diffusion: $(SD_LIB)

# Object files for daemon (separate C and C++ compilation)
DAEMON_C_OBJS = $(BUILD_DIR)/main.o $(BUILD_DIR)/socket.o $(BUILD_DIR)/protocol.o $(BUILD_DIR)/generate.o
DAEMON_CXX_OBJS = $(BUILD_DIR)/sd_wrapper.o

# Create build directory
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Compile C sources with gcc (C99 strict)
$(BUILD_DIR)/%.o: $(SRC_DIR)/%.c | $(BUILD_DIR)
	$(CC) $(CFLAGS) $(INCLUDES) $(SD_INCLUDES) -c $< -o $@

# Compile C++ sources with g++
$(BUILD_DIR)/sd_wrapper.o: $(SRC_DIR)/sd_wrapper.cpp | $(BUILD_DIR)
	$(CXX) $(CXXFLAGS) $(INCLUDES) $(SD_INCLUDES) -c $< -o $@

# Create bin directory
$(BIN_DIR):
	mkdir -p $(BIN_DIR)

# Main daemon binary (link with g++ for C++ standard library)
$(BIN_DIR)/weave-compute: $(DAEMON_C_OBJS) $(DAEMON_CXX_OBJS) $(SD_LIB) | $(BIN_DIR)
	$(CXX) -o $@ $(DAEMON_C_OBJS) $(DAEMON_CXX_OBJS) \
		$(SD_LIB) $(SD_GGML_LIBS) $(LDFLAGS) $(VULKAN_LDFLAGS)

.PHONY: test
test: $(TEST_DIR)/test_protocol $(TEST_DIR)/test_socket $(TEST_DIR)/test_sd_wrapper $(TEST_DIR)/test_generate
	@echo "Running unit tests..."
	@mkdir -p ./tmp
	@./$(TEST_DIR)/test_protocol
	@./$(TEST_DIR)/test_socket
	@./$(TEST_DIR)/test_sd_wrapper
	@./$(TEST_DIR)/test_generate

.PHONY: test-asan
test-asan: $(TEST_DIR)/test_protocol_asan $(TEST_DIR)/test_socket_asan
	@echo "Running tests with AddressSanitizer and UBSan..."
	@mkdir -p ./tmp
	@./$(TEST_DIR)/test_protocol_asan
	@./$(TEST_DIR)/test_socket_asan

.PHONY: test-stub
test-stub: $(TEST_DIR)/test_stub_generator

$(TEST_DIR)/test_protocol: $(TEST_DIR)/test_protocol.c $(SRC_DIR)/protocol.c
	$(CC) $(CFLAGS_DEBUG) $(INCLUDES) -o $@ $^ $(LDFLAGS)

$(TEST_DIR)/test_protocol_asan: $(TEST_DIR)/test_protocol.c $(SRC_DIR)/protocol.c
	$(CC) $(CFLAGS_ASAN) $(INCLUDES) -o $@ $^ $(LDFLAGS)

$(TEST_DIR)/test_socket: $(TEST_DIR)/test_socket.c $(SRC_DIR)/socket.c
	$(CC) $(CFLAGS_DEBUG) $(INCLUDES) -o $@ $^ $(LDFLAGS)

$(TEST_DIR)/test_socket_asan: $(TEST_DIR)/test_socket.c $(SRC_DIR)/socket.c
	$(CC) $(CFLAGS_ASAN) $(INCLUDES) -o $@ $^ $(LDFLAGS)

$(TEST_DIR)/test_stub_generator: $(TEST_DIR)/test_stub_generator.c $(SRC_DIR)/protocol.c
	$(CC) $(CFLAGS_DEBUG) $(INCLUDES) -o $@ $^ $(LDFLAGS)

$(TEST_DIR)/test_sd_wrapper: $(TEST_DIR)/test_sd_wrapper.c $(SRC_DIR)/sd_wrapper.cpp $(SD_LIB)
	$(CXX) $(CXXFLAGS_DEBUG) $(INCLUDES) $(SD_INCLUDES) -o $@ \
		$(TEST_DIR)/test_sd_wrapper.c $(SRC_DIR)/sd_wrapper.cpp \
		$(SD_LIB) $(SD_GGML_LIBS) $(LDFLAGS) $(VULKAN_LDFLAGS)

$(TEST_DIR)/test_generate: $(TEST_DIR)/test_generate.c $(SRC_DIR)/generate.c
	$(CC) $(CFLAGS_DEBUG) $(INCLUDES) -o $@ $^ $(LDFLAGS)

$(TEST_DIR)/test_stdin_monitor_unit: $(TEST_DIR)/test_stdin_monitor_unit.c $(SRC_DIR)/socket.c
	$(CC) $(CFLAGS_DEBUG) $(INCLUDES) -o $@ $^ $(LDFLAGS) -lpthread

# Benchmark targets
.PHONY: bench
bench: $(BENCH_DIR)/bench_generate
	@echo "Benchmark binary built: $(BENCH_DIR)/bench_generate"
	@echo ""
	@echo "To run benchmarks, you need:"
	@echo "  1. SD 3.5 Medium model file (see docs/DEVELOPMENT.md)"
	@echo "  2. GPU with Vulkan support"
	@echo ""
	@echo "Usage:"
	@echo "  ./$(BENCH_DIR)/bench_generate <model_path> [iterations]"
	@echo ""
	@echo "Example:"
	@echo "  ./$(BENCH_DIR)/bench_generate models/sd3.5_medium.safetensors 10"

# Benchmark object files (separate C and C++ compilation)
$(BUILD_DIR)/bench_generate.o: $(BENCH_DIR)/bench_generate.c | $(BUILD_DIR)
	$(CC) $(CFLAGS) $(INCLUDES) $(SD_INCLUDES) -c $< -o $@

$(BENCH_DIR)/bench_generate: $(BUILD_DIR)/bench_generate.o $(BUILD_DIR)/sd_wrapper.o $(SD_LIB)
	$(CXX) -o $@ $(BUILD_DIR)/bench_generate.o $(BUILD_DIR)/sd_wrapper.o \
		$(SD_LIB) $(SD_GGML_LIBS) $(LDFLAGS) $(VULKAN_LDFLAGS)

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
	rm -rf $(BUILD_DIR)
	rm -f $(TEST_DIR)/test_protocol $(TEST_DIR)/test_protocol_asan $(TEST_DIR)/test_stub_generator
	rm -f $(TEST_DIR)/test_socket $(TEST_DIR)/test_socket_asan
	rm -f $(TEST_DIR)/test_sd_wrapper
	rm -f $(TEST_DIR)/test_generate
	rm -f $(TEST_DIR)/test_stdin_monitor_unit
	rm -f $(BENCH_DIR)/bench_generate
	rm -f fuzz/fuzz_protocol fuzz/generate_corpus fuzz/test_corpus fuzz/stress_test
	rm -rf fuzz/corpus fuzz/crash-* fuzz/leak-* fuzz/timeout-*
	rm -rf ./tmp

.PHONY: clean-all
clean-all: clean
	@echo "Cleaning stable-diffusion.cpp build..."
	rm -rf $(SD_BUILD_DIR)

.PHONY: fmt
fmt:
	clang-format -i $(SRC_DIR)/*.c $(TEST_DIR)/*.c $(BENCH_DIR)/*.c include/**/*.h

.PHONY: fuzz
fuzz: fuzz/fuzz_protocol fuzz/corpus
	@echo "Running fuzzer for 60 seconds (1M+ iterations expected)..."
	@cd fuzz && ./fuzz_protocol corpus/ -max_total_time=60

.PHONY: fuzz-quick
fuzz-quick: fuzz/fuzz_protocol fuzz/corpus
	@echo "Running fuzzer for 10 seconds (quick smoke test)..."
	@cd fuzz && ./fuzz_protocol corpus/ -max_total_time=10

.PHONY: fuzz-long
fuzz-long: fuzz/fuzz_protocol fuzz/corpus
	@echo "Running fuzzer for 1 hour..."
	@cd fuzz && ./fuzz_protocol corpus/ -max_total_time=3600

fuzz/fuzz_protocol: fuzz/fuzz_protocol.c $(SRC_DIR)/protocol.c
	@echo "Building libFuzzer harness..."
	@which clang > /dev/null || (echo "Error: clang not found. Install with: sudo apt-get install clang" && exit 1)
	clang -fsanitize=fuzzer,address,undefined -O2 -g $(INCLUDES) -o $@ $^ $(LDFLAGS)

fuzz/generate_corpus: fuzz/generate_corpus.c
	$(CC) $(CFLAGS_DEBUG) $(INCLUDES) -o $@ $^ $(LDFLAGS)

fuzz/test_corpus: fuzz/test_corpus.c $(SRC_DIR)/protocol.c
	$(CC) $(CFLAGS_ASAN) $(INCLUDES) -o $@ $^ $(LDFLAGS)

fuzz/stress_test: fuzz/stress_test.c $(SRC_DIR)/protocol.c
	$(CC) $(CFLAGS_ASAN) $(INCLUDES) -o $@ $^ $(LDFLAGS)

fuzz/corpus: fuzz/generate_corpus
	@echo "Generating seed corpus..."
	@mkdir -p fuzz/corpus
	@cd fuzz && ./generate_corpus corpus/

.PHONY: test-corpus
test-corpus: fuzz/test_corpus fuzz/corpus
	@echo "Testing corpus files with ASan/UBSan..."
	@cd fuzz && ./test_corpus corpus/

.PHONY: stress-test
stress-test: fuzz/stress_test fuzz/corpus
	@echo "Running 1M iteration stress test (simulates fuzzing)..."
	@cd fuzz && ./stress_test corpus/ 1000000

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make all             - Build daemon and run tests (default)"
	@echo "  make bin/weave-compute - Build daemon binary"
	@echo "  make stable-diffusion - Build stable-diffusion.cpp library"
	@echo "  make test            - Build and run unit tests"
	@echo "  make test-asan       - Build and run tests with AddressSanitizer/UBSan"
	@echo "  make test-stub       - Build stub generator for integration tests"
	@echo "  make bench           - Build performance benchmark"
	@echo "  make test-corpus     - Test corpus files with ASan/UBSan"
	@echo "  make stress-test     - Run 1M iterations with ASan/UBSan (no clang needed)"
	@echo "  make fuzz            - Run fuzzer for 60 seconds (requires clang)"
	@echo "  make fuzz-quick      - Run fuzzer for 10 seconds (smoke test)"
	@echo "  make fuzz-long       - Run fuzzer for 1 hour (thorough test)"
	@echo "  make clean           - Remove build artifacts"
	@echo "  make clean-all       - Remove all build artifacts including stable-diffusion.cpp"
	@echo "  make fmt             - Format source code with clang-format"
	@echo "  make help            - Show this help message"
