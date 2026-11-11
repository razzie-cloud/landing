FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go index.html ./
COPY assets/icons/ ./assets/icons/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/render-landing .

FROM caddy:2-alpine
COPY --from=build /out/render-landing /usr/local/bin/render-landing
COPY Caddyfile /etc/caddy/Caddyfile
COPY landing.css /srv/
COPY assets/ /srv/assets/
COPY docker-entrypoint.sh /usr/local/bin/landing-entrypoint
ENTRYPOINT ["sh", "/usr/local/bin/landing-entrypoint"]
CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/ || exit 1
