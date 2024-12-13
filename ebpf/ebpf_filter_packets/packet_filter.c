#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <linux/if_packet.h>
#include <bpf/bpf_helpers.h>
#include "dependency_map.h"

// Define the dependency map
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, struct dependency_key);
    __type(value, __u8);
    __uint(max_entries, 256);
} dependency_map SEC(".maps");

// eBPF program attached to XDP hook
SEC("xdp")
int xdp_packet_filter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Parse Ethernet header
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) return XDP_DROP;

    // Filter only IPv4 packets
    if (eth->h_proto != __constant_htons(ETH_P_IP)) return XDP_PASS;

    // Parse IP header
    struct iphdr *ip = (struct iphdr *)(eth + 1);
    if ((void *)(ip + 1) > data_end) return XDP_DROP;

    // Create dependency map key
    struct dependency_key key = {
        .src_ip = ip->saddr,
        .dst_ip = ip->daddr,
    }; 

    // Check if the source-destination pair exists in the dependency map
    __u8 *value = bpf_map_lookup_elem(&dependency_map, &key);
    if (value) {
        // Allow the packet if the dependency exists
        bpf_printk("Packet allowed: ");
        return XDP_PASS;
    }

    // Drop the packet if no matching dependency is found
    bpf_printk("Packet dropped: ");
    return XDP_DROP;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";
