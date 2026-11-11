package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
