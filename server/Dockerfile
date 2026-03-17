FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/collector ./cmd/collector

FROM alpine:3.21
RUN adduser -D -H -u 10001 collector
USER collector
WORKDIR /app

COPY --from=build /out/collector /usr/local/bin/collector

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/collector"]
