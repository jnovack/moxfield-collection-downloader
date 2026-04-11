# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
WORKDIR /go/src/app

ENV USER=appuser
ENV UID=10001
ARG TARGETOS
ARG TARGETARCH
RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    "${USER}"

RUN --mount=type=cache,target=/var/cache/apk \
    apk update && apk add git ca-certificates

COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

ARG APPLICATION="mcd"
ARG BUILD_RFC3339="1970-01-01T00:00:00Z"
ARG PACKAGE="user/repo"
ARG REVISION="local"
ARG VERSION="dirty"
ARG GO_LDFLAGS="-w -s \
    -X main.buildRFC3339=${BUILD_RFC3339} \
    -X main.revision=${REVISION} \
    -X main.version=${VERSION} \
    "

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "${GO_LDFLAGS}" -o /go/bin/${APPLICATION} ./cmd/${APPLICATION}

FROM mcr.microsoft.com/playwright:v1.59.0-noble

ARG APPLICATION="mcd"
ARG BUILD_RFC3339="1970-01-01T00:00:00Z"
ARG DESCRIPTION="Moxfield Collection Downloader"
ARG PACKAGE="user/repo"
ARG REVISION="local"
ARG VERSION="dirty"

LABEL org.opencontainers.image.ref.name="${PACKAGE}" \
    org.opencontainers.image.created=$BUILD_RFC3339 \
    org.opencontainers.image.authors="Justin J. Novack <jnovack@gmail.com>" \
    org.opencontainers.image.documentation="https://github.com/${PACKAGE}/README.md" \
    org.opencontainers.image.description="${DESCRIPTION}" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.source="https://github.com/${PACKAGE}" \
    org.opencontainers.image.revision=$REVISION \
    org.opencontainers.image.version=$VERSION \
    org.opencontainers.image.url="https://hub.docker.com/r/${PACKAGE}/"

COPY --from=build /go/bin/${APPLICATION} /app

RUN useradd --uid 10001 --create-home --home-dir /home/appuser --shell /sbin/nologin appuser \
    && mkdir -p /data && chown appuser:appuser /data

USER appuser

WORKDIR /data

ENTRYPOINT ["/app"]
