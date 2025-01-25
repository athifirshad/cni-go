package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
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
	dependencyMap := []map[string]interface{}{}
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			var containerPort string
			for _, containerPortInfo := range container.Ports {
				containerPort = fmt.Sprintf("%d", containerPortInfo.ContainerPort)
				if containerPort != "" {
					break
				}
			}
			podIP := pod.Status.PodIP
			fmt.Printf("  Pod: %s, Container: %s, IP: %s\n", pod.Name, container.Name, podIP)
			depEntry := map[string]interface{}{
				"source_ip":       podIP,
				"destination_ips": []string{"any:any"}, // For now, default to "any:any"
			}
			dependencyMap = append(dependencyMap, depEntry)

			if _, ok := networkMap[pod.Name]; !ok {
				networkMap[pod.Name] = make(map[string]string)
			}
			networkMap[pod.Name][container.Name] = containerPort
		}
	}

	fmt.Println("\nGlobal container network map:")
	for podName, containers := range networkMap {
		fmt.Printf("  Pod: %s\n", podName)
		for containerName, ip := range containers {
			fmt.Printf("    Container: %s, IP: %s\n", containerName, ip)
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
			log.Printf("Pod added: %s, IP: %s", pod.Name, pod.Status.PodIP)
			depEntry := map[string]interface{}{
				"source_ip":       pod.Status.PodIP,
				"destination_ips": []string{"any:any"}, // For now, default to "any:any"
			}
			dependencyMap = append(dependencyMap, depEntry)
			for _, container := range pod.Spec.Containers {
				fmt.Printf("  Pod: %s, Container: %s, IP: %s\n", pod.Name, container.Name, pod.Status.PodIP)
			}
		case "MODIFIED":
			log.Printf("Pod modified: %s, IP: %s", pod.Name, pod.Status.PodIP)
			// When a pod is modified, we update its entry in the dependencyMap
			for i, entry := range dependencyMap {
				if entry["source_ip"] == pod.Status.PodIP {
					dependencyMap[i]["destination_ips"] = []string{"any:any"} // Update destination IPs
					break // Assuming source_ip is unique, we can break after finding the entry
				}
			}

		case "DELETED":
			log.Printf("Pod deleted: %s", pod.Name)
			// When a pod is deleted, remove its entry from dependencyMap
			for i, entry := range dependencyMap {
				if entry["source_ip"] == pod.Status.PodIP {
					dependencyMap = append(dependencyMap[:i], dependencyMap[i+1:]...) // Remove entry
					break
				}
			}
		case "ERROR":
			log.Printf("Error watching pods: %v", event.Object)
		}
	}
}
