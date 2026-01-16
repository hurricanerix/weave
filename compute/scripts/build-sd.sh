#!/bin/bash
#
# Build stable-diffusion.cpp with Vulkan backend
#
# This script builds stable-diffusion.cpp as a static library with Vulkan support.
# It's called by the Makefile during the build process.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SD_DIR="${SCRIPT_DIR}/../third_party/stable-diffusion.cpp"
BUILD_DIR="${SD_DIR}/build"

# Check if stable-diffusion.cpp exists
if [ ! -d "${SD_DIR}" ]; then
    echo "Error: stable-diffusion.cpp not found at ${SD_DIR}"
    echo "Run: git submodule update --init --recursive"
    exit 1
fi

# Create build directory
mkdir -p "${BUILD_DIR}"

# Configure with CMake
echo "Configuring stable-diffusion.cpp with Vulkan backend..."
cd "${BUILD_DIR}"

cmake .. \
    -DCMAKE_BUILD_TYPE=Release \
    -DSD_VULKAN=ON \
    -DSD_BUILD_SHARED_LIBS=OFF \
    -DSD_BUILD_EXAMPLES=OFF \
    -DCMAKE_POSITION_INDEPENDENT_CODE=ON

# Build
echo "Building stable-diffusion.cpp..."
cmake --build . --config Release -j$(nproc)

echo "stable-diffusion.cpp build complete"
echo "Library: ${BUILD_DIR}/libstable-diffusion.a"
