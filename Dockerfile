# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.24.2
ARG ALPINE_VERSION=3.21
ARG APP_UID=2000
ARG APP_GID=2000

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:${ALPINE_VERSION} AS runtime
ARG APP_UID
ARG APP_GID

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g ${APP_GID} app \
    && adduser -S -u ${APP_UID} -G app app

WORKDIR /app

COPY --from=builder --chmod=0555 /out/server /usr/local/bin/server

USER ${APP_UID}:${APP_GID}

EXPOSE 8080
ENV HTTP_ADDR=:8080

ENTRYPOINT ["/usr/local/bin/server"]
