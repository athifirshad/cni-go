#!/bin/bash
set -e

# Enhanced cleanup function
cleanup() {
    echo "Cleaning up..."
    
    # Delete namespaces
    sudo ip netns del ns1 2>/dev/null || true
    sudo ip netns del ns2 2>/dev/null || true
    
    # Clean CNI network data
    sudo rm -rf /var/lib/cni/networks/demo-network/
    
    # More aggressive veth cleanup
    echo "Cleaning up veth interfaces..."
    for veth in $(ip link show type veth | grep -o 'veth[[:alnum:]]*'); do
        echo "Removing veth interface: $veth"
        sudo ip link set "$veth" down 2>/dev/null || true
        sudo ip link delete "$veth" 2>/dev/null || true
    done
    
    # Wait for network subsystem to settle
    echo "Waiting for network cleanup..."
    sleep 3
    
    # Verify cleanup
    echo "Verifying cleanup..."
    ip link show type veth || true
}

# Trap to ensure cleanup on exit
trap cleanup EXIT

# Initial cleanup
cleanup

echo "Creating namespaces..."
sudo ip netns add ns1
sudo ip netns add ns2

# Add delay between namespace creation
sleep 1

echo "Debug: Listing network namespaces..."
ip netns list

echo "Debug: Listing all network interfaces before configuration..."
ip link show

echo "Configuring ns1..."
sudo env CNI_PATH=./bin CNI_CONTAINERID="ns1-$(date +%s)" /home/ubuntu/go/bin/cnitool add demo-network /var/run/netns/ns1

echo "Debug: Waiting for ns1 setup to complete..."
sleep 2

echo "Debug: Listing all network interfaces after ns1 configuration..."
ip link show

echo "Configuring ns2..."
sudo env CNI_PATH=./bin CNI_CONTAINERID="ns2-$(date +%s)" /home/ubuntu/go/bin/cnitool add demo-network /var/run/netns/ns2

echo "Debug: Listing all network interfaces after ns2 configuration..."
ip link show

# Test connectivity
echo "Testing connectivity..."
NS1_IP=$(sudo ip netns exec ns1 ip addr show dev eth0 | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)
NS2_IP=$(sudo ip netns exec ns2 ip addr show dev eth0 | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)

echo "NS1 IP: $NS1_IP"
echo "NS2 IP: $NS2_IP"

if [ ! -z "$NS1_IP" ] && [ ! -z "$NS2_IP" ]; then
    echo "Pinging from ns1 to ns2..."
    sudo ip netns exec ns1 ping -c 3 $NS2_IP

    echo "Pinging from ns2 to ns1..."
    sudo ip netns exec ns2 ping -c 3 $NS1_IP
else
    echo "Error: Could not get IP addresses"
    exit 1
fi