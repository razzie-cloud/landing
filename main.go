package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

//go:embed index.html assets/icons/*.svg
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

func run() error {
	domain := flag.String("domain", env("DOMAIN", "razzie.cloud"), "public base hostname")
	scheme := flag.String("scheme", env("SCHEME", "https"), "public URL scheme")
	port := flag.String("public-port", env("PUBLIC_PORT", ""), "optional public URL port")
	config := flag.String("apps-config", env("APPS_CONFIG_FILE", ""), "optional JSON app list")
	output := flag.String("output", "-", "output HTML path, or - for stdout")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	p, err := loadPage(*domain, *scheme, *port, *config)
	if err != nil {
		return err
	}
	html, err := render(p)
	if err != nil {
		return err
	}
	if *output == "-" {
		_, err = os.Stdout.Write(html)
		return err
	}
	return os.WriteFile(*output, html, 0644)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "landing:", err)
		os.Exit(1)
	}
}
