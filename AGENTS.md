# Landing development

Independent Git submodule of Razzie Cloud. Keep page, server, assets and image
work here; deployment configuration belongs in infra and parent integration in
the parent repository. Preserve the cloud logo and vendored PicoCSS license.

The Go HTTP service uses only the standard library. It embeds assets and renders
the page in memory at startup before listening on SERVICE_PORT (default 8080).
Sign in is built in; optional apps come from JSON.
Keep HTML template escaping, HTTP(S)-only links, validated hostname/port settings
and preset icon names. Configuration and source must never be exposed over HTTP.

Preserve GET/HEAD health endpoints, public-file routing without directory
listings, server timeouts, graceful shutdown and unprivileged container execution.

Run gofmt, go build ./..., go vet ./..., go test -race ./... and a Docker build
when available. Do not push or deploy without task authorization. Report checks and environment limitations.
