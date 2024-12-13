#ifndef DEPENDENCY_MAP_H
#define DEPENDENCY_MAP_H

#include <linux/types.h>

// Define the dependency map key structure
struct dependency_key {
    __u32 src_ip;  // Source IP address
    __u32 dst_ip;  // Destination IP address
};

#endif // DEPENDENCY_MAP_H
