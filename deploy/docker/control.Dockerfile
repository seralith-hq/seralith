# syntax=docker/dockerfile:1.7
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/ryan-tobin/new-startup/internal/version.Version=${VERSION}" \
    -o /out/sigil-control ./ee/cmd/sigil-control

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sigil-control /usr/local/bin/sigil-control
USER nonroot:nonroot
VOLUME ["/var/lib/sigil"]
ENV SIGIL_CONTROL_DB=/var/lib/sigil/control.db \
    SIGIL_CONTROL_ADDR=:8080 \
    SIGIL_CONTROL_JSON_LOGS=1
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/sigil-control"]
