package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildBlog(t *testing.T) {
	src := writeDocs(t, map[string]string{
		"README.md": "This note is not a post.\n",
		"older.md": "" +
			"---\n" +
			"title: Older note\n" +
			"date: 2026-11-01\n" +
			"summary: The earlier note.\n" +
			"---\n" +
			"\nSee [the newer note](newer.md) and the [threat model](/docs/security/threat-model).\n",
		"newer.md": "" +
			"---\n" +
			"title: Newer note\n" +
			"date: 2026-11-26\n" +
			"summary: The later note.\n" +
			"---\n" +
			"\nFirst paragraph.\n\n<script>alert(1)</script>\n\n" +
			"| Flag | Meaning |\n| :--- | ---: |\n| `--ws` | workspace |\n",
		"notes/nested.md": "" +
			"---\n" +
			"title: Zed note\n" +
			"date: 2026-11-26\n" +
			"summary: Same day, sorts by title after Newer note.\n" +
			"---\n" +
			"\nBody.\n",
	})
	out := filepath.Join(t.TempDir(), "site", "blog")
	if err := os.MkdirAll(filepath.Join(out, "stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := BuildBlog(src, out, "https://seralith.com/"); err != nil {
		t.Fatal(err)
	}

	index := read(t, filepath.Join(out, "index.html"))
	newerAt := strings.Index(index, "Newer note")
	nestedAt := strings.Index(index, "Zed note")
	olderAt := strings.Index(index, "Older note")
	if newerAt < 0 || nestedAt < 0 || olderAt < 0 || newerAt >= nestedAt || nestedAt >= olderAt {
		t.Fatalf("index order = newer %d, nested %d, older %d", newerAt, nestedAt, olderAt)
	}
	for _, want := range []string{
		`<title>Blog · Seralith</title>`,
		`<link rel="canonical" href="https://seralith.com/blog/" />`,
		`href="/blog/newer"`,
		`href="/blog/notes/nested"`,
		`<time datetime="2026-11-26">26 November 2026</time>`,
		`The later note.`,
		`aria-current="page">Blog</a>`,
		`<!--email_off-->`,
		`<!--/email_off-->`,
		blogIndexDesc,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("index.html missing %s", want)
		}
	}
	if strings.Contains(index, "This note is not a post") {
		t.Error("README.md was published as a post")
	}

	post := read(t, filepath.Join(out, "newer.html"))
	for _, want := range []string{
		`<title>Newer note · Seralith</title>`,
		`<link rel="canonical" href="https://seralith.com/blog/newer" />`,
		`<meta name="description" content="The later note." />`,
		`<h1>Newer note</h1>`,
		`First paragraph.`,
		`align="right"`,
		`<div class="table-scroll"><table>`,
		`href="` + repoBlob + `blog/newer.md"`,
		`<!--email_off-->`,
	} {
		if !strings.Contains(post, want) {
			t.Errorf("newer.html missing %s", want)
		}
	}
	if strings.Contains(post, "<script>") || strings.Contains(post, "style=") {
		t.Error("post contains a script or an inline style")
	}
	older := read(t, filepath.Join(out, "older.html"))
	for _, want := range []string{
		`href="/blog/newer"`,
		`href="/docs/security/threat-model"`,
	} {
		if !strings.Contains(older, want) {
			t.Errorf("older.html missing %s", want)
		}
	}
	if strings.Contains(post, "summary:") || strings.Contains(post, "date:") {
		t.Error("front matter was rendered")
	}

	if _, err := os.Stat(filepath.Join(out, "stale")); !os.IsNotExist(err) {
		t.Error("stale output was not removed")
	}
	if _, err := os.Stat(filepath.Join(out, "README.html")); !os.IsNotExist(err) {
		t.Error("README.md was rendered")
	}
	sitemap := read(t, filepath.Join(out, "sitemap.xml"))
	for _, want := range []string{
		"<loc>https://seralith.com/blog/</loc>",
		"<loc>https://seralith.com/blog/newer</loc>",
		"<loc>https://seralith.com/blog/notes/nested</loc>",
	} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap missing %s", want)
		}
	}
}

func TestBuildBlogRejectsPosts(t *testing.T) {
	cases := map[string]string{
		"no-matter.md":  "# Just a heading\n",
		"open.md":       "---\ntitle: T\n",
		"incomplete.md": "---\ntitle: T\ndate: 2026-11-26\n---\n",
		"bad-date.md":   "---\ntitle: T\ndate: November 26\nsummary: S\n---\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			src := writeDocs(t, map[string]string{name: body})
			out := filepath.Join(t.TempDir(), "blog")
			if err := BuildBlog(src, out, "https://seralith.com"); err == nil {
				t.Fatal("BuildBlog accepted a post without usable front matter")
			}
		})
	}
}

func TestBuildBlogRefusesDangerousOutput(t *testing.T) {
	src := writeDocs(t, map[string]string{
		"post.md": "---\ntitle: T\ndate: 2026-11-26\nsummary: S\n---\n\nBody.\n",
	})
	for _, out := range []string{"", ".", "/"} {
		if err := BuildBlog(src, out, "https://seralith.com"); err == nil {
			t.Errorf("BuildBlog accepted output %q", out)
		}
	}
}

func TestBuildBlogRequiresAPost(t *testing.T) {
	src := writeDocs(t, map[string]string{"README.md": "Not a post.\n"})
	if err := BuildBlog(src, filepath.Join(t.TempDir(), "blog"), "https://seralith.com"); err == nil {
		t.Fatal("BuildBlog accepted a directory with no posts")
	}
}
