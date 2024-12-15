
FROM golang:1.19-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o manager cmd/manager/main.go

FROM alpine:latest
COPY --from=builder /app/manager /usr/local/bin/
ENTRYPOINT ["manager"]