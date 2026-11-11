# Razzie Cloud landing

Independent source for `ghcr.io/razzie-cloud/landing`, included as the parent
checkout's `landing/` Git submodule. A standalone Go HTTP server embeds the page,
styles and assets in its binary. It reads config and renders the homepage once
at startup, then serves the result from memory. No browser JavaScript or database
is needed.
The design follows the browser's light/dark preference.

## Configuration

| Environment | Flag | Default |
| --- | --- | --- |
| `DOMAIN` | `--domain` | `razzie.cloud` |
| `SCHEME` | `--scheme` | `https` |
| `PUBLIC_PORT` | `--public-port` | no explicit port |
| `APPS_CONFIG_FILE` | `--apps-config` | no extra apps |
| `SERVICE_PORT` | `--port` | `8080` |
| `LOG_LEVEL` | — | `INFO` |

`DOMAIN` is a hostname without a scheme, port or path. It drives the canonical
URL, Open Graph metadata and built-in Sign in links at `auth.<DOMAIN>`.
Sign in always appears in navigation and as a card; it cannot be removed by config.
Flags override environment variables. `SERVICE_PORT` controls the HTTP listener;
`PUBLIC_PORT` only controls generated public URLs. TLS belongs to the ingress
proxy. `LOG_LEVEL` accepts DEBUG, INFO, WARN or ERROR; logs are JSON on stderr.
Restart after changing settings or config; no image rebuild is needed.

Copy [apps.example.json](apps.example.json) to your own config to list Files and
Notes. Each app needs `name`, exactly one of `subdomain` or absolute HTTP(S)
`url`, and optionally `description` and `icon`. Subdomains use the configured
domain, scheme and public port. External URLs keep their own scheme and port.
Apps appear in config order in both navigation and cards. All config text is
HTML-escaped. Invalid config, URLs and icon names fail startup.

Bundled icons: `app` (default), `files`, `notes`, `calendar`, `photos`, `music`,
`video`, `code`, `chat`, `globe`, `chart`, `settings`, `shield`.
See [assets/icons](assets/icons) to preview the SVGs. App definitions cannot
inject SVG or load remote icons. The cloud logo is the unchanged brand master;
the vendored PicoCSS license remains alongside its asset.

```sh
docker build -t landing:local .
docker run --rm -p 127.0.0.1:8766:8080 \
  -e DOMAIN=example.com -e APPS_CONFIG_FILE=/etc/landing/apps.json \
  --mount type=bind,src="$PWD/apps.example.json",dst=/etc/landing/apps.json,readonly \
  landing:local
```

The parent Compose stack uses `DOMAIN=localhost`, `SCHEME=http` and its shared
`LOCAL_PORT`. Swarm uses its `DOMAIN` and a Docker config containing Files/Notes.
Deployment files belong to the separate infra repository.

## Local preview and checks

```sh
go build ./...
go vet ./...
go test -race ./...
go run . --port 8766 --apps-config apps.example.json
```

Use `--domain localhost --scheme http --public-port 8080` for local app links.
Open <http://localhost:8766/> to preview. The page, `/landing.css` and `/assets/*`
accept GET/HEAD requests. Other paths and directory listings return 404; source
and config files are never served. GET/HEAD `/healthz` and `/readyz` return 200
after startup succeeds. Invalid config prevents the server from listening.
SIGINT/SIGTERM drain active HTTP requests with a ten-second shutdown deadline.
The container runs as an unprivileged user and needs no writable web directory.

## Releases

Tag pushes publish a container to GHCR via GitHub Actions. Publish a new image
before selecting its `LANDING_IMAGE_TAG` in infra. The extracted release tags
preserve the historical Files-only page and the later addition of Notes.
Historical screenshots under `previews/` are retained as design references.
