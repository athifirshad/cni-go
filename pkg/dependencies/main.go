package main

import (
	"context"
	"log"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	// Configure logging with microsecond precision
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	// Get hostname (pod name)
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Error getting hostname: %v", err)
	}

	// Create kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Error creating k8s client config: %v", err)
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Error creating k8s client: %v", err)
		return
	}

	// Continuous logging loop
	for {
		log.Printf("=== CNI Manager Status Report ===")
		log.Printf("Pod Name: %s", hostname)

		// Get node name from environment
		nodeName := os.Getenv("NODE_NAME")
		log.Printf("Running on Node: %s", nodeName)

		if clientset != nil {
			// Get node info
			node, err := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
			if err != nil {
				log.Printf("Error getting node info: %v", err)
			} else {
				log.Printf("Node Labels: %v", node.Labels)
				log.Printf("Node CPU: %v", node.Status.Capacity.Cpu())
				log.Printf("Node Memory: %v", node.Status.Capacity.Memory())
			}

			// List pods on this node
			pods, err := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{
				FieldSelector: "spec.nodeName=" + nodeName,
			})
			if err != nil {
				log.Printf("Error listing pods: %v", err)
			} else {
				log.Printf("Total pods on node: %d", len(pods.Items))
				for _, pod := range pods.Items {
					log.Printf("Pod: %s/%s (Phase: %s)",
						pod.Namespace,
						pod.Name,
						pod.Status.Phase)
				}
			}
		}

		// log.Printf("BPF Map Status: %s", BPFMapPath)
		log.Printf("============================")

		// Sleep for 30 seconds before next update
		time.Sleep(30 * time.Second)
	}
}
