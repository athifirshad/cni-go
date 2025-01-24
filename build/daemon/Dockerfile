FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY . .

# Build CNI daemon only
RUN CGO_ENABLED=0 GOOS=linux go build -o /daemon ./cmd/daemon/main.go

FROM alpine:3.18

RUN apk add --no-cache iproute2 iptables sudo

# Add sudo configuration
RUN echo "daemon ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/daemon && \
    chmod 0440 /etc/sudoers.d/daemon

USER root

COPY --from=builder /daemon /usr/local/bin/cni-daemon

ENTRYPOINT ["/usr/local/bin/cni-daemon"]
