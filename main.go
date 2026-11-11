package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed index.html landing.css assets
var assets embed.FS

var label = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

type app struct {
	Name        string `json:"name"`
	Subdomain   string `json:"subdomain,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
	Host        string `json:"-"`
}
type page struct {
	Origin, AuthOrigin, AuthHost string
	Apps                         []app
}

func hostname(s string) bool {
	if len(s) > 253 {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if !label.MatchString(part) {
			return false
		}
	}
	return true
}

func loadPage(domain, scheme, port, config string) (page, error) {
	var p page
	if !hostname(domain) {
		return p, errors.New("DOMAIN must be a hostname without a scheme, port, or path")
	}
	if scheme != "http" && scheme != "https" {
		return p, errors.New("SCHEME must be http or https")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return p, errors.New("PUBLIC_PORT must be between 1 and 65535")
		}
	}
	origin := func(host string) string {
		if port != "" {
			host = net.JoinHostPort(host, port)
		}
		return scheme + "://" + host
	}
	p.Origin, p.AuthOrigin = origin(domain), origin("auth."+domain)
	p.AuthHost = strings.TrimPrefix(p.AuthOrigin, scheme+"://")
	if config == "" {
		return p, nil
	}
	f, err := os.Open(config)
	if err != nil {
		return p, fmt.Errorf("open apps config: %w", err)
	}
	defer f.Close()
	var cfg struct {
		Apps []app `json:"apps"`
	}
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return p, fmt.Errorf("decode apps config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return p, errors.New("apps config must contain one JSON object")
	}
	if len(cfg.Apps) > 64 {
		return p, errors.New("apps config supports at most 64 apps")
	}
	for i, a := range cfg.Apps {
		if strings.TrimSpace(a.Name) == "" || (a.Subdomain == "") == (a.URL == "") {
			return p, fmt.Errorf("app %d needs a name and exactly one of subdomain or url", i+1)
		}
		if a.Subdomain != "" {
			if !label.MatchString(a.Subdomain) {
				return p, fmt.Errorf("app %d has an invalid subdomain", i+1)
			}
			a.URL = origin(a.Subdomain+"."+domain) + "/"
		}
		u, err := url.Parse(a.URL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return p, fmt.Errorf("app %d needs an absolute http(s) URL without credentials", i+1)
		}
		a.Host = u.Host
		if a.Icon == "" {
			a.Icon = "app"
		}
		if !label.MatchString(a.Icon) {
			return p, fmt.Errorf("app %d has an unknown icon", i+1)
		}
		if _, err := assets.ReadFile("assets/icons/" + a.Icon + ".svg"); err != nil {
			return p, fmt.Errorf("app %d has an unknown icon", i+1)
		}
		p.Apps = append(p.Apps, a)
	}
	return p, nil
}

func render(p page) ([]byte, error) {
	t, err := template.ParseFS(assets, "index.html")
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, p); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// Render once before accepting requests. Only public assets are served from the
// embedded filesystem; the HTML template and operator configuration stay private.
func newHandler(p page) (http.Handler, error) {
	html, err := render(p)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	home := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(html))
	}
	mux.HandleFunc("GET /{$}", home)
	mux.HandleFunc("GET /index.html", home)
	files := http.FileServer(http.FS(assets))
	mux.Handle("GET /landing.css", files)
	mux.HandleFunc("GET /assets/", func(w http.ResponseWriter, r *http.Request) {
		info, err := fs.Stat(assets, strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
	for _, path := range []string{"/healthz", "/readyz"} {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = io.WriteString(w, "ok\n")
			}
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'self'; base-uri 'none'; form-action 'none'")
		mux.ServeHTTP(w, r)
	}), nil
}

func serve(ctx context.Context, server *http.Server) error {
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		if err := <-result; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func run() error {
	domain := flag.String("domain", env("DOMAIN", "razzie.cloud"), "public base hostname")
	scheme := flag.String("scheme", env("SCHEME", "https"), "public URL scheme")
	port := flag.String("public-port", env("PUBLIC_PORT", ""), "optional public URL port")
	config := flag.String("apps-config", env("APPS_CONFIG_FILE", ""), "optional JSON app list")
	listenPort := flag.String("port", env("SERVICE_PORT", "8080"), "HTTP listen port")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	portNumber, err := strconv.Atoi(*listenPort)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("SERVICE_PORT must be between 1 and 65535")
	}
	p, err := loadPage(*domain, *scheme, *port, *config)
	if err != nil {
		return err
	}
	handler, err := newHandler(p)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(portNumber)),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("Server starting", "event", "server.starting", "address", server.Addr)
	return serve(ctx, server)
}

func main() {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(env("LOG_LEVEL", "INFO")))); err != nil {
		fmt.Fprintln(os.Stderr, "landing: invalid LOG_LEVEL")
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	if err := run(); err != nil {
		slog.Error("Service failed", "event", "server.failed", "error", err)
		os.Exit(1)
	}
}
