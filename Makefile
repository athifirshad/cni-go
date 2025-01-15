.PHONY: all build clean create load-image

all: build

build: build-plugin build-manager

build-plugin:
	sudo go build -o bin/demo-cni-plugin cmd/main.go

build-manager:
	sudo go build -o bin/cni-manager cmd/manager/main.go

clean:
	rm -f bin/*

create:
	sudo docker build -t cni-manager:latest .
