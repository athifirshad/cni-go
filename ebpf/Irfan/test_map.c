#include <bpf/libbpf.h>
#include <stdio.h>
#include <arpa/inet.h>
#include <linux/bpf.h> 

int main() {
    int map_fd = bpf_obj_get("/sys/fs/bpf/allowed_sources");
    if (map_fd < 0) {
        perror("bpf_obj_get");
        return 1;
    }

    unsigned char mac[6] = {0x01, 0x02, 0x03, 0x04, 0x05, 0x06};
    __be32 ip = htonl(0xc0a80001); // 192.168.0.1 in network byte order

    if (bpf_map_update_elem(map_fd, &mac, &ip, BPF_ANY) != 0) {
        perror("bpf_map_update_elem");
        return 1;
    }

    printf("Entry added: MAC=01:02:03:04:05:06, IP=192.168.0.1\n");
    return 0;
}
