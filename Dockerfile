FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY . .

# Build CNI daemon
RUN CGO_ENABLED=0 GOOS=linux go build -o /daemon ./cmd/daemon/main.go

# Build manager
RUN CGO_ENABLED=0 GOOS=linux go build -o /manager ./cmd/manager/main.go

FROM alpine:3.18

RUN apk add --no-cache iproute2 iptables

COPY --from=builder /daemon /usr/local/bin/cni-daemon
COPY --from=builder /manager /usr/local/bin/cni-manager
COPY demo-config.json /etc/cni/net.d/10-demo.conflist

ENTRYPOINT ["/usr/local/bin/cni-manager"]
