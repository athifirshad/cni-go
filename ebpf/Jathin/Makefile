obj-m := ebpf_print_metadata.o

KERNEL_HEADERS := /usr/src/linux-headers-$(shell uname -r)
CFLAGS_ebpf_print_metadata.o := -Wall -Werror -O2 -g -target bpf -D__TARGET_ARCH_arm64 -I/usr/include -I../../include/uapi -I$(KERNEL_HEADERS)/arch/arm64/include -I$(KERNEL_HEADERS)/arch/arm64/include/asm -I$(KERNEL_HEADERS)/include

all: build

build:
	clang $(CFLAGS_ebpf_print_metadata.o) -c ebpf_print_metadata.c -o ebpf_print_metadata.o

clean:
	rm -f *.o
