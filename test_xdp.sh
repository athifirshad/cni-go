#!/bin/bash

# Compile the program
make clean && make

# Check if compilation was successful
if [ ! -f xdp_filter.o ]; then
    echo "Compilation failed!"
    exit 1
fi

# Load the program (replace eth0 with your interface)
echo "Loading XDP program..."
ip link set dev eth0 xdp obj xdp_filter.o sec xdp

# Watch for debug output
echo "Watching for debug output (press Ctrl+C to stop)..."
cat /sys/kernel/debug/tracing/trace_pipe

# Cleanup function
cleanup() {
    echo "Removing XDP program..."
    ip link set dev eth0 xdp off
}

# Set cleanup on script exit
trap cleanup EXIT