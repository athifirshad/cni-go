// Enhanced Source Verification Code Using eBPF/XDP
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/udp.h>
#include <linux/tcp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
cd

// Define a map to hold allowed source MAC and IP addresses
struct source_entry {
    __be32 ip; // IPv4 address
    unsigned char mac[ETH_ALEN]; // MAC address
};

struct bpf_map_def SEC("maps") allowed_sources = {
    _uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, unsigned char[ETH_ALEN]); // MAC address as key
    __type(value, __be32); // IPv4 address as value
} allowed_sources SEC("maps");


// Define a map for session tracking to prevent packet injection
struct session_entry {
    __be32 src_ip;
    __be32 dest_ip;
    __u16 src_port;
    __u16 dest_port;
    __u8 protocol;
};

struct bpf_map_def SEC("maps") session_map = {


    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 1024);
    __type(key, struct session_entry); // Session details as key
    __type(value, __u8); // Placeholder value
} session_map SEC("maps");


SEC("xdp")
int xdp_source_verification(struct xdp_md *ctx) {
    // Parse packet headers
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    // Ensure packet is large enough for Ethernet header
    struct ethhdr *eth = data;
    if (data + sizeof(*eth) > data_end) {
        return XDP_PASS; // Let the packet pass if malformed
    }

    // Check if the packet is IPv4
    if (eth->h_proto != __constant_htons(ETH_P_IP)) {
        return XDP_PASS;
    }

    // Ensure packet is large enough for IP header
    struct iphdr *ip = data + sizeof(*eth);
    if ((void *)ip + sizeof(*ip) > data_end) {
        return XDP_PASS;
    }

    // Verify source MAC and IP
    __be32 *allowed_ip = bpf_map_lookup_elem(&allowed_sources, eth->h_source);
    if (!allowed_ip || *allowed_ip != ip->saddr) {
        return XDP_DROP; // Drop the packet if source verification fails
    }

    // Handle TCP/UDP session tracking
    if (ip->protocol == IPPROTO_TCP || ip->protocol == IPPROTO_UDP) {
        struct session_entry session = {};
        struct tcphdr *tcp;
        struct udphdr *udp;

        if (ip->protocol == IPPROTO_TCP) {
            tcp = (void *)ip + (ip->ihl * 4);
            if ((void *)tcp + sizeof(*tcp) > data_end) {
                return XDP_PASS;
            }
            session.src_port = tcp->source;
            session.dest_port = tcp->dest;
        } else {
            udp = (void *)ip + (ip->ihl * 4);
            if ((void *)udp + sizeof(*udp) > data_end) {
                return XDP_PASS;
            }
            session.src_port = udp->source;
            session.dest_port = udp->dest;
        }

        session.src_ip = ip->saddr;
        session.dest_ip = ip->daddr;
        session.protocol = ip->protocol;

        // Check if the session is already tracked
        __u8 *value = bpf_map_lookup_elem(&session_map, &session);
        if (!value) {
            // New session: add it to the map
            __u8 placeholder = 1;
            bpf_map_update_elem(&session_map, &session, &placeholder, BPF_ANY);
        }
    }

    return XDP_PASS; // Allow verified packets
}

char _license[] SEC("license") = "GPL";
