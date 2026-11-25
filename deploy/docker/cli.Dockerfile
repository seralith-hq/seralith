# syntax=docker/dockerfile:1.7
FROM node:26-alpine AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist/ ./internal/webui/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/seralith-hq/seralith/internal/version.Version=${VERSION}" \
    -o /out/seralith ./cmd/seralith

FROM alpine:3.24
RUN apk add --no-cache git ca-certificates && adduser -D -u 10001 seralith
COPY --from=build /out/seralith /usr/local/bin/seralith
USER seralith
WORKDIR /workspace
ENTRYPOINT ["seralith"]
