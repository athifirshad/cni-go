source_verification.o: %.o: %.c
	clang \
		-target bpf \
		-I/usr/include/$(shell uname -m)-linux-gnu \
		-I/usr/include/bpf \
		-g \
		-O2 -c $< -o $@
