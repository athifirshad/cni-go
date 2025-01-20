#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/udp.h>
#include <linux/tcp.h>
#include <arpa/inet.h>

SEC("xdp")
int xdp_print_metadata(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Print ingress interface index and RX queue index
    bpf_printk("Ingress Interface Index: %u, RX Queue Index: %u", ctx->ingress_ifindex, ctx->rx_queue_index);

    struct ethhdr *eth = data;
    if ((void*)eth + sizeof(*eth) > data_end) {
        return XDP_PASS;
    }

    if (ntohs(eth->h_proto) == ETH_P_IP) {
        struct iphdr *iph = data + sizeof(*eth);
        if ((void*)iph + sizeof(*iph) > data_end) {
            return XDP_PASS;
        }

        __u32 saddr_n = iph->saddr;
        __u32 daddr_n = iph->daddr;

        char saddr_str[INET_ADDRSTRLEN];
        char daddr_str[INET_ADDRSTRLEN];

        inet_ntop(AF_INET, &saddr_n, saddr_str, INET_ADDRSTRLEN);
        inet_ntop(AF_INET, &daddr_n, daddr_str, INET_ADDRSTRLEN);

        bpf_printk("IP Packet: Source IP: %s, Destination IP: %s", saddr_str, daddr_str);

        if (iph->protocol == IPPROTO_UDP) {
            struct udphdr *udph = (void*)iph + sizeof(*iph);
            if ((void*)udph + sizeof(*udph) <= data_end) {
                bpf_printk("  UDP: Source Port: %u, Destination Port: %u", ntohs(udph->source), ntohs(udph->dest));
            }
        } else if (iph->protocol == IPPROTO_TCP) {
            struct tcphdr *tcph = (void*)iph + sizeof(*iph);
            if ((void*)tcph + sizeof(*tcph) <= data_end) {
                bpf_printk("  TCP: Source Port: %u, Destination Port: %u", ntohs(tcph->source), ntohs(tcph->dest));
            }
        }
    }

    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
