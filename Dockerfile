FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /cni-manager cmd/manager/main.go

FROM alpine:latest
COPY --from=builder /cni-manager /usr/local/bin/
RUN chmod +x /usr/local/bin/cni-manager

ENTRYPOINT ["/usr/local/bin/cni-manager"]