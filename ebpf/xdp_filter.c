// xdp_filter.c

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <linux/if_packet.h>
#include <bpf/bpf_helpers.h>
#include "dependency_map.h"

// Statistics map to track packets
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 4);
    __type(key, __u32);
    __type(value, __u64);
} stats_map SEC(".maps");

// Dependency map for allowed connections
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, struct dependency_key);
    __type(value, __u8);
    __uint(max_entries, 10000);
} dependency_map SEC(".maps");

// Stats indices
#define STAT_TOTAL     0
#define STAT_ALLOWED   1
#define STAT_DROPPED   2
#define STAT_INVALID   3

static __always_inline
void update_stats(__u32 stat_type) {
    __u64 *value;
    value = bpf_map_lookup_elem(&stats_map, &stat_type);
    if (value)
        __sync_fetch_and_add(value, 1);
}

SEC("xdp")
int xdp_packet_filter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Update total packets stat
    update_stats(STAT_TOTAL);

    // Parse Ethernet header
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        update_stats(STAT_INVALID);
        return XDP_DROP;
    }

    // Only process IPv4 packets
    if (eth->h_proto != __constant_htons(ETH_P_IP)) {
        return XDP_PASS;
    }

    // Parse IP header
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        update_stats(STAT_INVALID);
        return XDP_DROP;
    }

    // Create dependency map key
    struct dependency_key key = {
        .src_ip = ip->saddr,
        .dst_ip = ip->daddr,
    };

    // Log packet info
    bpf_printk("Packet: src_ip=0x%x dst_ip=0x%x\n", 
               key.src_ip, 
               key.dst_ip);

    // Check dependency map
    __u8 *allowed = bpf_map_lookup_elem(&dependency_map, &key);
    if (allowed) {
        update_stats(STAT_ALLOWED);
        bpf_printk("Packet allowed\n");
        return XDP_PASS;
    }

    // Packet not allowed
    update_stats(STAT_DROPPED);
    bpf_printk("Packet dropped\n");
    return XDP_DROP;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";