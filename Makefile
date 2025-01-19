# Makefile for building and deploying the CNI manager

# Image name and tag
IMAGE_NAME := cni-manager
IMAGE_TAG := latest
REGISTRY := localhost:5000

# Build the Docker image
build:
	sudo docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	sudo docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	sudo docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

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
	sudo docker rmi $(IMAGE_NAME):$(IMAGE_TAG) || true

debug-bpf:
	@echo "Manager pod logs:"
	@kubectl logs -n kube-system $$(kubectl get pods -n kube-system | grep cni-manager | awk '{print $$1}') -c manager || true

logs:
	kubectl logs -n kube-system $$(kubectl get pods -n kube-system | grep cni-manager | awk '{print $$1}') -f

coredns-logs:
	kubectl logs -n kube-system -l k8s-app=kube-dns
	
prune:
	sudo docker image prune 

.PHONY: build deploy clean revive debug-bpf prune logs coredns-logs
