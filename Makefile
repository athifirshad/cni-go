build:
	go build -o bin/demo-cni-plugin ./cmd/main.go

clean:
	rm -f bin/mycniplugin
# Get logs from manager pod
manager-logs:
	kubectl logs -n kube-system deployment/cni-manager --container manager --tail=100 -f

# Get logs from daemon pods
daemon-logs:
	kubectl logs -n kube-system daemonset/cni-daemon --container cni-daemon --tail=100 -f

# Get logs from specific daemon pod (usage: make daemon-logs-one POD=pod-name)
daemon-logs-one:
	kubectl logs -n kube-system $(POD) -c cni-daemon --tail=100 -f

# Get all CNI related events
events:
	kubectl get events -n kube-system | grep -E 'cni-manager|cni-daemon'

# Get pod IDs
get-ids:
	@echo "Manager pods:"
	@kubectl get pods -n kube-system | grep cni-manager
	@echo "\nDaemon pods:"
	@kubectl get pods -n kube-system | grep cni-daemon

# Get total count of manager pods
count-managers:
	@echo "Total cni-manager pods:"
	@kubectl get pods -n kube-system | grep cni-manager | wc -l

redeploy:
	sudo docker build -t localhost:5000/cni-manager:latest -f build/manager/Dockerfile .
	sudo docker build -t localhost:5000/cni-daemon:latest -f build/daemon/Dockerfile .
	kubectl delete -f deploy/manager.yaml
	kubectl delete -f deploy/daemon.yaml
	kubectl apply -f deploy/manager.yaml
	kubectl apply -f deploy/daemon.yaml

revive:
	kubectl delete -f deploy/manager.yaml
	kubectl delete -f deploy/daemon.yaml
	kubectl apply -f deploy/manager.yaml
	kubectl apply -f deploy/daemon.yaml

# Clean docker cache
docker-clean:
	sudo docker system prune -af
	sudo docker builder prune -af
	sudo docker image prune -af

# Delete all CNI manager pods
clean-managers:
	kubectl delete pods -n kube-system -l app=cni-manager --force --grace-period=0
	kubectl delete pods -n kube-system --field-selector=status.phase==Failed --force --grace-period=0

# Clean everything including pods
clean-all: clean docker-clean clean-managers

# Restart Kubernetes
kube-restart:
	sudo systemctl restart kubelet
	sudo systemctl restart containerd
	sleep 30
	kubectl get nodes

# Complete system cleanup
nuke:
	# Delete all pods in kube-system namespace
	kubectl delete pods --all -n kube-system --force --grace-period=0
	# # Delete evicted pods
	# kubectl get pods -A | grep Evicted | awk '{print $$2}' | xargs kubectl delete pod -n kube-system
	# Clear logs
	sudo journalctl --vacuum-time=1s

	# Remove CNI state
	sudo rm -rf /var/run/cni/
	sudo rm -rf /var/log/containers/*
	sudo rm -rf /var/log/pods/*
	
	# Restart kubelet to ensure clean state
	sudo systemctl restart kubelet

.PHONY: build clean manager-logs daemon-logs daemon-logs-one events redeploy get-ids count-managers docker-clean clean-all clean-managers kube-restart nuke
