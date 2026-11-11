FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go index.html landing.css ./
COPY assets/ ./assets/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/landing .

FROM alpine:3.22
RUN addgroup -g 10001 landing && adduser -D -u 10001 -G landing landing
COPY --from=build /out/landing /usr/local/bin/landing
USER landing
ENV SERVICE_PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:${SERVICE_PORT:-8080}/readyz || exit 1
ENTRYPOINT ["/usr/local/bin/landing"]
