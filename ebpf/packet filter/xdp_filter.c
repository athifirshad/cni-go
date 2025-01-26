// xdp_filter.c

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <linux/if_packet.h>
#include <bpf/bpf_helpers.h>
#include "dependency_map.h"

// Hardcoded IPs
#define POD2_IP 0x0AF40018  // 10.244.0.24
#define POD3_IP 0x0AF40017  // 10.244.0.23

// Dependency map
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, struct dependency_key);
    __type(value, __u8);
    __uint(max_entries, 10000);
} dependency_map SEC(".maps");

SEC("xdp")
int xdp_packet_filter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Parse Ethernet header
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        bpf_printk("Invalid ethernet header\n");
        return XDP_DROP;
    }

    // Only process IPv4 packets
    if (eth->h_proto != __constant_htons(ETH_P_IP)) {
        return XDP_PASS;
    }

    // Parse IP header
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        bpf_printk("Invalid IP header\n");
        return XDP_DROP;
    }

    // Debug print the IPs
    bpf_printk("Packet: src=0x%x dst=0x%x\n", ip->saddr, ip->daddr);

    // Only allow 10.244.0.23 -> 10.244.0.24
    if (ip->saddr == POD3_IP && ip->daddr == POD2_IP) {
        bpf_printk("Allowed: 10.244.0.23 -> 10.244.0.24\n");
        return XDP_PASS;
    }

    bpf_printk("Dropped: not allowed path\n");
    return XDP_DROP;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";