build:
	go build -o bin/demo-cni-plugin ./cmd/main.go

clean:
	rm -f bin/mycniplugin