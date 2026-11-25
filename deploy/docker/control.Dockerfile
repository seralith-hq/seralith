# syntax=docker/dockerfile:1.7
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/seralith-hq/seralith/internal/version.Version=${VERSION}" \
    -o /out/seralith-control ./ee/cmd/seralith-control

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/seralith-control /usr/local/bin/seralith-control
USER nonroot:nonroot
VOLUME ["/var/lib/seralith"]
ENV SERALITH_CONTROL_DB=/var/lib/seralith/control.db \
    SERALITH_CONTROL_ADDR=:8080 \
    SERALITH_CONTROL_JSON_LOGS=1
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/seralith-control"]
