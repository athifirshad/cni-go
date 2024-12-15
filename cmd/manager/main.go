package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/athifirshad/go-cni/pkg/dependencies"
)

func main() {
	manager, err := dependencies.NewManager()
	if err != nil {
		log.Fatalf("Failed to create manager: %v", err)
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
