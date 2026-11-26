package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDocs(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRewrite(t *testing.T) {
	b := &builder{pages: map[string]*page{}}
	for _, rel := range []string{"README.md", "cli.md", "spec/policy.md", "spec/README.md"} {
		p := &page{Rel: rel}
		p.URL, p.out = locate(rel)
		b.pages[rel] = p
	}
	cases := []struct{ from, dest, want string }{
		{"README.md", "cli.md", "/docs/cli"},
		{"README.md", "spec/policy.md#effects", "/docs/spec/policy#effects"},
		{"spec/policy.md", "../cli.md", "/docs/cli"},
		{"spec/policy.md", "README.md", "/docs/spec/"},
		{"cli.md", "spec", "/docs/spec/"},
		{"cli.md", "README.md", "/docs/"},
		{"README.md", "rfcs/0000-template.md", repoBlob + "docs/rfcs/0000-template.md"},
		{"README.md", "../ee/README.md", repoBlob + "ee/README.md"},
		{"spec/policy.md", "../../LICENSE", repoBlob + "LICENSE"},
		{"cli.md", "#flags", "#flags"},
		{"cli.md", "https://example.com/x.md", "https://example.com/x.md"},
		{"cli.md", "mailto:security@seralith.com", "mailto:security@seralith.com"},
		{"cli.md", "/security", "/security"},
	}
	for _, c := range cases {
		if got := b.rewrite(c.from, c.dest); got != c.want {
			t.Errorf("rewrite(%q, %q) = %q, want %q", c.from, c.dest, got, c.want)
		}
	}
}

func TestBuild(t *testing.T) {
	src := writeDocs(t, map[string]string{
		"README.md":             "# Seralith Documentation\n\n- [Getting started](getting-started.md)\n- [Policy](spec/policy.md#effects)\n- [Template](rfcs/0000-template.md)\n",
		"getting-started.md":    "# Getting started\n\nInstall Seralith and run your first gated commit.\n\n<script>alert(1)</script>\n\n| Flag | Meaning |\n| :--- | ---: |\n| `--ws` | workspace |\n",
		"spec/policy.md":        "# Policy language\n\n## Effects\n\nSee the [CLI](../getting-started.md) and [LICENSE](../../LICENSE).\n",
		"rfcs/0000-template.md": "# RFC template\n",
	})
	out := filepath.Join(t.TempDir(), "site", "docs")
	if err := os.MkdirAll(filepath.Join(out, "stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Build(src, out, "https://seralith.com/"); err != nil {
		t.Fatal(err)
	}

	index := read(t, filepath.Join(out, "index.html"))
	for _, want := range []string{
		`href="/docs/getting-started"`,
		`href="/docs/spec/policy#effects"`,
		`href="` + repoBlob + `docs/rfcs/0000-template.md"`,
		`<title>Seralith Documentation · Seralith docs</title>`,
		`<link rel="canonical" href="https://seralith.com/docs/" />`,
		`aria-current="page">Overview</a>`,
		`<!--email_off-->`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("index.html missing %s", want)
		}
	}

	gs := read(t, filepath.Join(out, "getting-started.html"))
	if strings.Contains(gs, "<script>") {
		t.Error("raw HTML in Markdown was rendered")
	}
	if strings.Contains(gs, "style=") {
		t.Error("inline styles would be blocked by the site's content policy")
	}
	if !strings.Contains(gs, `<meta name="description" content="Install Seralith and run your first gated commit." />`) {
		t.Error("description not taken from the first paragraph")
	}
	if !strings.Contains(gs, `align="right"`) {
		t.Error("table alignment not rendered as an attribute")
	}
	if !strings.Contains(gs, `<div class="table-scroll"><table>`) {
		t.Error("tables are not wrapped for horizontal scrolling")
	}

	policy := read(t, filepath.Join(out, "spec", "policy.html"))
	for _, want := range []string{`id="effects"`, `href="/docs/getting-started"`, `href="` + repoBlob + `LICENSE"`, "<p class=\"docs-nav-title\">Specifications</p>"} {
		if !strings.Contains(policy, want) {
			t.Errorf("spec/policy.html missing %s", want)
		}
	}

	for _, gone := range []string{"rfcs", "stale"} {
		if _, err := os.Stat(filepath.Join(out, gone)); !os.IsNotExist(err) {
			t.Errorf("%s should not exist in the output", gone)
		}
	}
	sitemap := read(t, filepath.Join(out, "sitemap.xml"))
	for _, want := range []string{"<loc>https://seralith.com/docs/</loc>", "<loc>https://seralith.com/docs/spec/policy</loc>"} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap missing %s", want)
		}
	}
}

func TestBuildRefusesDangerousOutput(t *testing.T) {
	src := writeDocs(t, map[string]string{"README.md": "# Docs\n"})
	for _, out := range []string{"", ".", "/"} {
		if err := Build(src, out, "https://seralith.com"); err == nil {
			t.Errorf("Build accepted output %q", out)
		}
	}
}
