#ifndef DEPENDENCY_MAP_H
#define DEPENDENCY_MAP_H

#include <linux/types.h>

struct dependency_key {
    __u32 src_ip;
    __u32 dst_ip;
};

#endif