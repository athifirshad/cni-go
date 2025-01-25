FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY . .

# Build manager only
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/athifirshad/go-cni/pkg/version.GitCommit=$(git rev-parse HEAD)" \
    -o /manager ./cmd/manager/main.go

FROM alpine:3.18

RUN apk add --no-cache sudo

# Add sudo configuration
RUN echo "manager ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/manager && \
    chmod 0440 /etc/sudoers.d/manager

USER root

COPY --from=builder /manager /usr/local/bin/cni-manager

ENTRYPOINT ["/usr/local/bin/cni-manager"]
