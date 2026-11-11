package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPService(t *testing.T) {
	p, err := loadPage("localhost", "http", "9191", "apps.example.json")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/index.html", "/landing.css", "/assets/pico.min.css", "/assets/razzie-cloud-logo.png", "/assets/icons/notes.svg", "/assets/PICO-LICENSE.md", "/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			get := httptest.NewRecorder()
			handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, path, nil))
			if get.Code != http.StatusOK || get.Body.Len() == 0 {
				t.Fatalf("GET %s: %d %s", path, get.Code, get.Body.String())
			}
			if get.Header().Get("X-Content-Type-Options") != "nosniff" || get.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing response protection")
			}
			head := httptest.NewRecorder()
			handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, path, nil))
			if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Type") != get.Header().Get("Content-Type") {
				t.Fatalf("HEAD %s: %d %s", path, head.Code, head.Body.String())
			}
			if path == "/" || path == "/index.html" {
				if !strings.Contains(get.Header().Get("Content-Type"), "text/html") || !strings.Contains(get.Body.String(), `href="http://notes.localhost:9191/"`) || strings.Contains(get.Body.String(), "{{") {
					t.Fatal("homepage was not rendered with startup configuration")
				}
			}
			post := httptest.NewRecorder()
			handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, path, nil))
			if post.Code != http.StatusMethodNotAllowed {
				t.Fatalf("POST %s: %d", path, post.Code)
			}
		})
	}
	for _, path := range []string{"/missing", "/assets/", "/assets/icons/", "/assets/missing.svg", "/main.go", "/README.md", "/apps.example.json", "/etc/landing/apps.json", "/assets/%2e%2e/index.html"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("private or missing path %s: %d", path, w.Code)
		}
	}
	// Embedded files retain normal HTTP types and byte-range support.
	r := httptest.NewRequest(http.MethodGet, "/assets/razzie-cloud-logo.png", nil)
	r.Header.Set("Range", "bytes=0-7")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusPartialContent || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("image range: %d %v", w.Code, w.Header())
	}
}

func TestHTTPServiceKeepsStartupConfiguration(t *testing.T) {
	file := configFile(t, `{"apps":[{"name":"Startup app","subdomain":"app"}]}`)
	p, err := loadPage("example.com", "https", "", file)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("invalid after startup"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Startup app") {
		t.Fatal("requests should use the configuration rendered at startup")
	}
}

func configFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOptionalAppsAndDomain(t *testing.T) {
	for _, file := range []string{"", "apps.example.json"} {
		p, err := loadPage("localhost", "http", "9191", file)
		if err != nil {
			t.Fatal(err)
		}
		b, err := render(p)
		if err != nil {
			t.Fatal(err)
		}
		html := string(b)
		for _, want := range []string{`href="http://auth.localhost:9191/"`, `<strong>Sign in</strong>`, `href="http://localhost:9191/"`, `content="http://localhost:9191/assets/razzie-cloud-logo.png"`} {
			if !strings.Contains(html, want) {
				t.Fatalf("missing %s", want)
			}
		}
		if strings.Contains(html, "razzie.cloud") {
			t.Fatal("default domain leaked")
		}
		if file == "" && (len(p.Apps) != 0 || strings.Contains(html, ">Files<")) {
			t.Fatal("optional apps present without config")
		}
		if file != "" {
			if len(p.Apps) != 2 || p.Apps[0].URL != "http://files.localhost:9191/" || p.Apps[1].URL != "http://notes.localhost:9191/" {
				t.Fatal(p.Apps)
			}
			if !strings.Contains(html, `assets/icons/notes.svg`) {
				t.Fatal("missing preset")
			}
		}
	}
}

func TestExternalAppEscapingAndFallbackIcon(t *testing.T) {
	file := configFile(t, `{"apps":[{"name":"<script>alert(1)</script>","description":"<b>text</b>","url":"https://external.example/path?a=1&b=2"}]}`)
	p, err := loadPage("example.com", "https", "", file)
	if err != nil {
		t.Fatal(err)
	}
	b, err := render(p)
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	if strings.Contains(html, "<script>") || strings.Contains(html, "<b>text") || !strings.Contains(html, "&lt;script&gt;") || !strings.Contains(html, `assets/icons/app.svg`) || !strings.Contains(html, `https://external.example/path?a=1&amp;b=2`) {
		t.Fatal(html)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, args := range [][3]string{{"bad/path", "https", ""}, {"-bad.example", "https", ""}, {"example.com", "ftp", ""}, {"example.com", "https", "0"}, {"example.com", "https", "65536"}, {"", "https", ""}} {
		if _, err := loadPage(args[0], args[1], args[2], ""); err == nil {
			t.Fatal("accepted", args)
		}
	}
	for _, cfg := range []string{
		`{`, `{"apps":[]} {}`, `{"app":[]}`, `{"apps":[{"name":"bad"}]}`,
		`{"apps":[{"name":"bad","subdomain":"files","url":"https://example.com"}]}`,
		`{"apps":[{"name":"bad","subdomain":"../x"}]}`,
		`{"apps":[{"name":"bad","url":"javascript:alert(1)"}]}`,
		`{"apps":[{"name":"bad","url":"//example.com"}]}`,
		`{"apps":[{"name":"bad","url":"https://u:p@example.com"}]}`,
		`{"apps":[{"name":"bad","subdomain":"files","icon":"../notes"}]}`,
		`{"apps":[{"name":"bad","subdomain":"files","icon":"unknown"}]}`,
	} {
		if _, err := loadPage("example.com", "https", "", configFile(t, cfg)); err == nil {
			t.Fatal("accepted", cfg)
		}
	}
	if _, err := loadPage("example.com", "https", "", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing config accepted")
	}
}
