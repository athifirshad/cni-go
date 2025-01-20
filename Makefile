# Makefile for building and deploying the CNI manager

# Image name and tag
IMAGE_NAME := cni-manager
IMAGE_TAG := latest

# Build the Docker image
build:
	sudo docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

# Deploy the CNI manager
deploy: build
	kubectl apply -f deploy/rbac.yaml
	kubectl apply -f deploy/daemonset.yaml

revive:
	kubectl delete -f deploy/daemonset.yaml || true
	sudo rm -f /sys/fs/bpf/container_deps || true
	sudo rm -f /var/run/cni/manager.sock || true
	sleep 5
	kubectl apply -f deploy/daemonset.yaml
	kubectl get pods -n kube-system | grep cni-manager

# Clean up
clean:
	kubectl delete -f deploy/daemonset.yaml || true
	sudo rm -f /sys/fs/bpf/container_deps || true
	sudo rm -f /var/run/cni/manager.sock || true
	docker rmi cni-manager:latest || true

.PHONY: build deploy clean
