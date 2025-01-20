package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/athifirshad/go-cni/pkg/dependencies"
)

func main() {
	// Ensure BPF filesystem is mounted
	if err := os.MkdirAll("/sys/fs/bpf", 0755); err != nil {
		log.Fatalf("Failed to create BPF filesystem directory: %v", err)
	}

	// Create manager
	log.Println("Starting manager...")
	manager, err := dependencies.NewManager()
	if err != nil {
		log.Fatalf("Failed to create manager: %v", err)
	}

	// Create socket directory
	if err := os.MkdirAll(filepath.Dir(dependencies.ManagerSocket), 0755); err != nil {
		log.Fatalf("Failed to create socket directory: %v", err)
	}

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		if err := os.Remove(dependencies.ManagerSocket); err != nil {
			log.Printf("Failed to remove socket: %v", err)
		}
		os.Exit(0)
	}()

	if err := manager.Start(); err != nil {
		log.Fatalf("Manager failed: %v", err)
	}
}
