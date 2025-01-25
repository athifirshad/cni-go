package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Update the key structure to include ports
type MapKey struct {
	SrcIP   uint32 `json:"src_ip"`   // network byte order
	DstIP   uint32 `json:"dst_ip"`   // network byte order
	SrcPort uint16 `json:"src_port"` // network byte order
	DstPort uint16 `json:"dst_port"` // network byte order
}

// Update value to be simple integer
type MapValue struct {
	Value uint32
}

// Add function to convert string IP to uint32
func ipToUint32(ipStr string) (uint32, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0, fmt.Errorf("invalid IP: %s", ipStr)
	}
	ipv4 := ip.To4()
	if ipv4 == nil {
		return 0, fmt.Errorf("not an IPv4 address: %s", ipStr)
	}
	return binary.BigEndian.Uint32(ipv4), nil
}

// Convert uint16 to network byte order
func htons(port uint16) uint16 {
	return (port<<8)&0xff00 | port>>8
}

// Convert uint16 from network byte order to host byte order
func ntohs(port uint16) uint16 {
	return (port<<8)&0xff00 | port>>8
}

// Update the map update function to include ports
func updateBPFMapEntry(bpfMap *ebpf.Map, srcIP string, srcPort uint16, dstIP string, dstPort uint16) error {
	srcIPInt, err := ipToUint32(srcIP)
	if err != nil {
		return fmt.Errorf("invalid source IP: %v", err)
	}

	dstIPInt, err := ipToUint32(dstIP)
	if err != nil {
		return fmt.Errorf("invalid destination IP: %v", err)
	}

	key := MapKey{
		SrcIP:   srcIPInt,
		DstIP:   dstIPInt,
		SrcPort: htons(srcPort), // Convert to network byte order
		DstPort: htons(dstPort), // Convert to network byte order
	}

	value := MapValue{
		Value: 1, // Default allow
	}

	return bpfMap.Update(&key, &value, ebpf.UpdateAny)
}

// Update print function to show ports
func printBPFMapContents(bpfMap *ebpf.Map) {
	entries := []map[string]interface{}{}

	var key MapKey
	var value MapValue

	iter := bpfMap.Iterate()
	for iter.Next(&key, &value) {
		entry := map[string]interface{}{
			"key": map[string]interface{}{
				"src_ip":   key.SrcIP,
				"dst_ip":   key.DstIP,
				"src_port": ntohs(key.SrcPort), // Convert back to host byte order
				"dst_port": ntohs(key.DstPort),
			},
			"value": value.Value,
		}
		entries = append(entries, entry)
	}

	jsonBytes, err := json.MarshalIndent(entries, "", "    ")
	if err != nil {
		fmt.Printf("Error marshaling map contents: %v\n", err)
		return
	}
	fmt.Printf("\nBPF Map Contents:\n%s\n", string(jsonBytes))
}

func main() {
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatalf("rlimit.RemoveMemlock() failed: %v", err)
	}
	// var kubeconfig *string
	// if home := homedir.HomeDir(); home != "" {
	// 	kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	// } else {
	// 	kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	// }
	// flag.Parse()
	kubeconfig := flag.String("kubeconfig", "/home/ubuntu/.kube/config", "(optional) absolute path to the kubeconfig file") // Hardcoded path
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		// If kubeconfig is not provided, try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Fatalf("failed to create kubernetes config: %v", err)
		}
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to create kubernetes clientset: %v", err)
	}

	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list pods: %v", err)
	}

	networkMap := make(map[string]map[string]string)
	dependencyMap := []map[string]string{}
	for _, pod := range pods.Items {
		if pod.Namespace != "kube-system" {
			log.Printf("Processing pod %s in namespace %s", pod.Name, pod.Namespace)
			log.Printf("Pod phase: %s, Pod IP: %s", pod.Status.Phase, pod.Status.PodIP)

			// Only process pods that are running and have an IP
			if pod.Status.Phase != corev1.PodRunning {
				log.Printf("Skipping pod %s: not in Running phase", pod.Name)
				continue
			}

			if pod.Status.PodIP == "" {
				log.Printf("Skipping pod %s: no IP assigned yet", pod.Name)
				continue
			}

			for _, container := range pod.Spec.Containers {
				var containerPort string
				for _, containerPortInfo := range container.Ports {
					containerPort = fmt.Sprintf("%d", containerPortInfo.ContainerPort)
					if containerPort != "" {
						break
					}
				}
				podIP := pod.Status.PodIP
				if podIP != "" { // Check if PodIP is not empty
					fmt.Printf("  Pod: %s, Container: %s, IP: %s\n", pod.Name, container.Name, podIP)
					depEntry := map[string]string{
						"source":      fmt.Sprintf("%s:%s", podIP, containerPort),
						"destination": "any:any",
					}
					dependencyMap = append(dependencyMap, depEntry)

					if _, ok := networkMap[pod.Name]; !ok {
						networkMap[pod.Name] = make(map[string]string)
					}
					networkMap[pod.Name][container.Name] = podIP
				}
			}
		}
	}

	fmt.Println("\nGlobal container network map:")
	for podName, containers := range networkMap {
		fmt.Printf("  Pod: %s\n", podName)
		for containerName, ip := range containers {
			fmt.Printf("    Container: %s, IP: %s\n", containerName, ip) // Print only IP
		}
	}

	// Write maps to file
	file, err := os.Create("container_maps.json")
	if err != nil {
		log.Fatalf("failed to create container_maps.json: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(map[string]interface{}{
		"networkMap":    networkMap,
		"dependencyMap": dependencyMap,
	})
	if err != nil {
		log.Fatalf("failed to encode maps to json: %v", err)
	}

	// --- eBPF Map Creation and Population ---

	// 1. Create a map spec with struct key and value
	mapSpec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    uint32(binary.Size(MapKey{})),
		ValueSize:  uint32(binary.Size(MapValue{})),
		MaxEntries: 128,
	}

	// 2. Create the eBPF map
	dependencyBPFMap, err := ebpf.NewMap(mapSpec)
	if err != nil {
		log.Fatalf("failed to create eBPF map: %v", err)
	}
	defer dependencyBPFMap.Close()

	// Only populate BPF map with entries from dependencyMap (user containers)
	for _, depEntry := range dependencyMap {
		sourceStr := depEntry["source"]
		if sourceStr == "" || strings.Contains(sourceStr, "any") {
			continue // Skip invalid or wildcard entries
		}

		ipStr, portStr, err := net.SplitHostPort(sourceStr)
		if err != nil {
			log.Printf("Failed to parse source address '%s': %v", sourceStr, err)
			continue
		}

		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			log.Printf("Failed to parse port '%s': %v", portStr, err)
			continue
		}

		// For now, use a default destination IP (can be updated based on your requirements)
		dstIP := "0.0.0.0"

		if err := updateBPFMapEntry(dependencyBPFMap, ipStr, uint16(port), dstIP, 0); err != nil {
			log.Printf("Failed to update BPF map: %v", err)
			continue
		}
		log.Printf("Added to BPF map: Source IP: %s, Port: %d", ipStr, port)
	}
	printBPFMapContents(dependencyBPFMap)

	// Watch for pod changes
	watch, err := clientset.CoreV1().Pods("").Watch(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("failed to watch pods: %v", err)
	}
	defer watch.Stop()

	ch := watch.ResultChan()
	for event := range ch {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			log.Printf("unexpected type: %v", event.Object)
			continue
		}

		switch event.Type {
		case "ADDED":
			if pod.Status.Phase == corev1.PodRunning && pod.Status.PodIP != "" && pod.Namespace != "kube-system" {
				// Only add to BPF map if it's in our networkMap
				if _, exists := networkMap[pod.Name]; exists {
					log.Printf("Adding user pod to BPF map: %s, IP: %s", pod.Name, pod.Status.PodIP)
					for _, container := range pod.Spec.Containers {
						for _, port := range container.Ports {
							if err := updateBPFMapEntry(dependencyBPFMap, pod.Status.PodIP, uint16(port.ContainerPort), "0.0.0.0", 0); err != nil {
								log.Printf("Failed to update BPF map: %v", err)
								continue
							}
							log.Printf("Added container port to BPF map - Pod: %s, Container: %s, IP: %s, Port: %d",  // Changed %s to %d
								pod.Name, container.Name, pod.Status.PodIP, port.ContainerPort)
						}
					}
					printBPFMapContents(dependencyBPFMap)
				}
			}
		case "MODIFIED":
			log.Printf("Pod modified: %s, IP: %s", pod.Name, pod.Status.PodIP)
			if pod.Status.Phase == corev1.PodRunning && pod.Status.PodIP != "" {
				// Clear old entries for this IP
				lookup := &MapKey{}
				var value MapValue
				entries := dependencyBPFMap.Iterate()
				for entries.Next(&lookup, &value) {
					ipBytes := make([]byte, 4)
					binary.BigEndian.PutUint32(ipBytes, lookup.SrcIP)
					if net.IP(ipBytes).String() == pod.Status.PodIP {
						dependencyBPFMap.Delete(lookup)
					}
				}

				// Add new entries
				for _, container := range pod.Spec.Containers {
					for _, port := range container.Ports {
						if err := updateBPFMapEntry(dependencyBPFMap, pod.Status.PodIP, uint16(port.ContainerPort), "0.0.0.0", 0); err != nil {
							log.Printf("Failed to update BPF map for modified pod: %v", err)
						}
					}
					printBPFMapContents(dependencyBPFMap)
				}
			}
		case "DELETED":
			log.Printf("Pod deleted: %s", pod.Name)
			// When a pod is deleted, remove its entry from dependencyMap
			for i, entry := range dependencyMap {
				if entry["source"] == fmt.Sprintf("%s:any", pod.Status.PodIP) {
					dependencyMap = append(dependencyMap[:i], dependencyMap[i+1:]...)
					break
				}
			}
			printBPFMapContents(dependencyBPFMap)
		case "ERROR":
			log.Printf("Error watching pods: %v", event.Object)
		}
	}
}
