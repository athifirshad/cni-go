#include <bpf/bpf.h>
#include <bpf/libbpf.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <unistd.h>
#include <arpa/inet.h>     // For htonl
#include <net/if.h>        // For if_nametoindex
#include <linux/if_link.h> // For XDP_FLAGS_UPDATE_IF_NOEXIST
#include "dependency_map.h" // Include the header for the map structure (dependency_key)

#define BPF_OBJECT_FILE "packet_filter.o"  // The compiled BPF object file

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Usage: %s <interface>\n", argv[0]);
        return 1;
    }

    const char *interface = argv[1];
    struct bpf_object *obj;
    int prog_fd, map_fd;

    // Step 1: Load eBPF program and object file
    obj = bpf_object__open_file(BPF_OBJECT_FILE, NULL);
    if (!obj) {
        perror("Failed to open eBPF object file");
        return 1;
    }

    if (bpf_object__load(obj)) {
        perror("Failed to load eBPF object");
        return 1;
    }

    // Step 2: Get program file descriptor
    prog_fd = bpf_program__fd(bpf_object__find_program_by_title(obj, "xdp"));
    if (prog_fd < 0) {
        perror("Failed to get program FD");
        return 1;
    }
    
    int bridge_index = if_nametoindex("br0");


    // Step 3: Attach XDP program to network interface
   /* if (bpf_set_link_xdp_fd(if_nametoindex(interface), prog_fd, XDP_FLAGS_UPDATE_IF_NOEXIST) < 0) {
        perror("Failed to attach XDP program");
        return 1;
    }*/
    
    if (bpf_set_link_xdp_fd(bridge_index, prog_fd, XDP_FLAGS_UPDATE_IF_NOEXIST) < 0) {
    perror("Failed to attach XDP program");
    return 1;
   }

    // Step 4: Retrieve map file descriptor using map name from the loaded BPF object
    struct bpf_map *map = bpf_object__find_map_by_name(obj, "dependency_map");
    if (!map) {
        perror("Failed to find map in the eBPF object");
        return 1;
    }

    // Get the file descriptor for the map
    map_fd = bpf_map__fd(map);
    if (map_fd < 0) {
        perror("Failed to get map FD");
        return 1;
    }

    // Step 5: Add an example dependency (e.g., IPs of containers or hosts)
   
    struct dependency_key key1= {
    .src_ip = htonl(0x0A000001),  // 10.0.0.1
    .dst_ip = htonl(0x0A000002),  // 10.0.0.2
};

  struct dependency_key key2= {
    .src_ip = htonl(0x0A000002),  // 10.0.0.2
    .dst_ip = htonl(0x0A000001),  // 10.0.0.1
};


    __u8 value = 1;  // Mark as an active dependency

    if (bpf_map_update_elem(map_fd, &key1, &value, BPF_ANY) < 0) {
        perror("Failed to update dependency 1 in map");
        return 1;
    }

    printf("Dependency added: 10.0.0.1 -> 10.0.0.2\n");
    
    if (bpf_map_update_elem(map_fd, &key2, &value, BPF_ANY) < 0) {
        perror("Failed to update dependency 2 in map");
        return 1;
    }

    printf("Dependency added: 10.0.0.2 -> 10.0.0.1 \n");

    // Step 6: Monitor the packet processing indefinitely
    while (1) {
        // Look up the packet value from the map (just an example)
        __u8 count = 0;
        if (bpf_map_lookup_elem(map_fd, &key1, &count) == 0) {
            printf("Packet count for 10.0.0.1 -> 10.0.0.2: %u\n", count);
        } else {
            printf("Failed to read packet count\n");
        }
        sleep(1);
    }

    return 0;
}
