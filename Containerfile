# syntax=docker/dockerfile:1

FROM docker.io/golang:1.25-alpine AS builder

WORKDIR /bethrou

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -trimpath -ldflags "-s -w" -o /out/bethrou ./cmd/bethrou

FROM docker.io/alpine:3.22

RUN apk add --no-cache ca-certificates

RUN mkdir -p /etc/bethrou

COPY --from=builder /out/bethrou /usr/bin/bethrou
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 1080

VOLUME ["/etc/bethrou"]

ENTRYPOINT ["/entrypoint.sh"]
