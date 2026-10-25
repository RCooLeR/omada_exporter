# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS builder

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

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

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
