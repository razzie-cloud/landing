# Landing development

Independent Git submodule of Razzie Cloud. Keep page, renderer, assets and image
work here; deployment configuration belongs in infra and parent integration in
the parent repository. Preserve the cloud logo and vendored PicoCSS license.

The Go renderer uses only the standard library. Caddy serves its startup output
and local static assets. Sign in is built in; optional apps come from JSON.
Keep HTML template escaping, HTTP(S)-only links, validated hostname/port settings
and preset icon names. Configuration and source must stay outside the web root.

Run gofmt, go build ./..., go vet ./..., go test -race ./..., sh -n
docker-entrypoint.sh and a Docker build when available. Do not push or deploy
without task authorization. Report checks and environment limitations.
