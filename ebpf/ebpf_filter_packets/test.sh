# Create a virtual bridge
sudo ip link add name br0 type bridge

# Bring the bridge interface up
sudo ip link set dev br0 up

# Create two network namespaces
sudo ip netns add ns1
sudo ip netns add ns2
sudo ip netns add ns3


# Create a veth pair between ns1 and br0
sudo ip link add veth1 type veth peer name veth1-br

# Create a veth pair between ns2 and br0
sudo ip link add veth2 type veth peer name veth2-br

sudo ip link add veth3 type veth peer name veth3-br

# Move veth1 to ns1
sudo ip link set veth1 netns ns1

# Move veth2 to ns2
sudo ip link set veth2 netns ns2

sudo ip link set veth3 netns ns3

# Assign IP addresses inside ns1
sudo ip netns exec ns1 ip addr add 10.0.0.1/24 dev veth1
sudo ip netns exec ns1 ip link set dev veth1 up

# Assign IP addresses inside ns2
sudo ip netns exec ns2 ip addr add 10.0.0.2/24 dev veth2
sudo ip netns exec ns2 ip link set dev veth2 up

sudo ip netns exec ns3 ip addr add 10.0.0.3/24 dev veth3
sudo ip netns exec ns3 ip link set dev veth3 up

# Add veth1-br and veth2-br to the bridge
sudo ip link set dev veth1-br master br0
sudo ip link set dev veth2-br master br0
sudo ip link set dev veth3-br master br0

# Bring up the bridge-facing interfaces
sudo ip link set dev veth1-br up
sudo ip link set dev veth2-br up
sudo ip link set dev veth3-br up

sudo sysctl -w net.ipv4.ip_forward=1

