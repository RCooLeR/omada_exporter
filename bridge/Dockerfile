# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine3.24@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

RUN apk upgrade --no-cache \
    && apk add --no-cache git

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags="-s -w -X github.com/RCooLeR/omada_exporter/cmd.version=${VERSION}" \
      -o /out/omada-exporter .

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 omada \
    && adduser -S -D -H -u 10001 -G omada omada

COPY --link --chmod=0755 --from=builder /out/omada-exporter /usr/bin/omada-exporter

LABEL org.opencontainers.image.source="https://github.com/RCooLeR/omada_exporter" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="Omada Exporter"

USER 10001:10001
EXPOSE 9202

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -T 5 -O /dev/null "http://127.0.0.1:${OMADA_PORT:-9202}/healthz" || exit 1

ENTRYPOINT ["/usr/bin/omada-exporter"]
