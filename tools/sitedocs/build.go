package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

const repoBlob = "https://github.com/seralith-hq/seralith/blob/main/"

var excludedDirs = map[string]bool{"rfcs": true}

type section struct {
	key, name string
}

var sections = []section{
	{"", "Get started"},
	{"features", "Features"},
	{"integrations", "Integrations"},
	{"spec", "Specifications"},
	{"security", "Security and compliance"},
	{"compliance", "Security and compliance"},
	{"operations", "Operations"},
}

var rootOrder = map[string]int{"README.md": 0, "getting-started.md": 1, "architecture.md": 2, "cli.md": 3}

type page struct {
	Rel       string
	URL       string
	Title     string
	NavTitle  string
	Desc      string
	Section   string
	HTML      template.HTML
	Date      string
	DateLabel string
	Summary   string
	out       string
	source    []byte
	when      time.Time
}

type navSection struct {
	Name  string
	Pages []*page
}

type builder struct {
	src, out, base string
	pages          map[string]*page
	md             goldmark.Markdown
}

// Build renders every Markdown file under src into HTML pages under out, along with
// a sitemap. Links between docs become site links; links to other repository files
// point at GitHub.
func Build(src, out, base string) error {
	if out == "" || filepath.Clean(out) == "." || filepath.Clean(out) == "/" {
		return fmt.Errorf("refusing to write to %q", out)
	}
	b := &builder{
		src:   src,
		out:   out,
		base:  strings.TrimSuffix(base, "/"),
		pages: map[string]*page{},
		md:    newMarkdown(),
	}
	if err := b.collect(); err != nil {
		return err
	}
	if len(b.pages) == 0 {
		return errors.New("no Markdown files found")
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	nav := b.nav()
	for _, p := range b.pages {
		if err := b.render(p); err != nil {
			return fmt.Errorf("%s: %w", p.Rel, err)
		}
	}
	for _, p := range b.pages {
		if err := b.write(p, nav); err != nil {
			return fmt.Errorf("%s: %w", p.Rel, err)
		}
	}
	return b.sitemap()
}

func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
			extension.Strikethrough,
			extension.Linkify,
			extension.TaskList,
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
}

// walkMarkdown walks src with os.Root and calls fn for every Markdown file.
// skip drops a file, or a whole directory when the entry is a directory.
func walkMarkdown(src string, skip func(rel string, d fs.DirEntry) bool, fn func(rel string, data []byte) error) error {
	root, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer root.Close()
	return fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skip != nil && skip(rel, d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(rel, ".md") {
			return nil
		}
		data, err := root.ReadFile(rel)
		if err != nil {
			return err
		}
		return fn(rel, data)
	})
}

func (b *builder) collect() error {
	return walkMarkdown(b.src, func(rel string, d fs.DirEntry) bool {
		return d.IsDir() && excludedDirs[rel]
	}, func(rel string, data []byte) error {
		pg := &page{Rel: rel, source: data, Section: sectionName(rel)}
		pg.URL, pg.out = locate(rel)
		pg.Title = title(data, rel)
		pg.NavTitle = pg.Title
		if rel == "README.md" {
			pg.NavTitle = "Overview"
		}
		pg.Desc = description(data)
		b.pages[rel] = pg
		return nil
	})
}

func locate(rel string) (urlPath, file string) {
	slug := strings.TrimSuffix(rel, ".md")
	switch {
	case slug == "README":
		return "/docs/", "index.html"
	case path.Base(slug) == "README":
		dir := path.Dir(slug)
		return "/docs/" + dir + "/", dir + "/index.html"
	default:
		return "/docs/" + slug, slug + ".html"
	}
}

func sectionName(rel string) string {
	dir, _, found := strings.Cut(rel, "/")
	if !found {
		dir = ""
	}
	for _, s := range sections {
		if s.key == dir {
			return s.name
		}
	}
	return strings.ToUpper(dir[:1]) + strings.ReplaceAll(dir[1:], "-", " ")
}

func title(src []byte, rel string) string {
	for line := range strings.SplitSeq(string(src), "\n") {
		if t, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return strings.TrimSuffix(path.Base(rel), ".md")
}

var (
	mdLink   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdMarker = regexp.MustCompile("[*_`]")
)

func description(src []byte) string {
	var parts []string
	for line := range strings.SplitSeq(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		if strings.ContainsAny(line[:1], "#|-<>`*!") || strings.HasPrefix(line, "1.") {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
	}
	d := mdMarker.ReplaceAllString(mdLink.ReplaceAllString(strings.Join(parts, " "), "$1"), "")
	if len(d) > 160 {
		d = d[:157]
		if i := strings.LastIndex(d, " "); i > 100 {
			d = d[:i]
		}
		d += "…"
	}
	return d
}

func (b *builder) nav() []navSection {
	bySection := map[string][]*page{}
	for _, p := range b.pages {
		bySection[p.Section] = append(bySection[p.Section], p)
	}
	var order []string
	seen := map[string]bool{}
	for _, s := range sections {
		if !seen[s.name] {
			seen[s.name] = true
			order = append(order, s.name)
		}
	}
	var extra []string
	for name := range bySection {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	var out []navSection
	for _, name := range append(order, extra...) {
		pages := bySection[name]
		if len(pages) == 0 {
			continue
		}
		sort.Slice(pages, func(i, j int) bool {
			oi, iok := rootOrder[pages[i].Rel]
			oj, jok := rootOrder[pages[j].Rel]
			switch {
			case iok && jok:
				return oi < oj
			case iok != jok:
				return iok
			default:
				return pages[i].NavTitle < pages[j].NavTitle
			}
		})
		out = append(out, navSection{Name: name, Pages: pages})
	}
	return out
}

func renderMarkdown(md goldmark.Markdown, source []byte, rewrite func(dest string) string) (string, error) {
	doc := md.Parser().Parse(text.NewReader(source))
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			v.Destination = []byte(rewrite(string(v.Destination)))
		case *ast.Image:
			v.Destination = []byte(rewrite(string(v.Destination)))
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		return "", err
	}
	html := strings.NewReplacer("<table>", `<div class="table-scroll"><table>`, "</table>", "</table></div>").Replace(buf.String())
	return html, nil
}

func (b *builder) render(p *page) error {
	html, err := renderMarkdown(b.md, p.source, func(dest string) string {
		return b.rewrite(p.Rel, dest)
	})
	if err != nil {
		return err
	}
	p.HTML = template.HTML(html) //nolint:gosec // goldmark escapes raw HTML because the unsafe option is off
	return nil
}

// rewrite maps a link in the Markdown file at fromRel to its address on the site.
func (b *builder) rewrite(fromRel, dest string) string {
	if dest == "" || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "/") {
		return dest
	}
	if u, err := url.Parse(dest); err != nil || u.Scheme != "" {
		return dest
	}
	target, frag, _ := strings.Cut(dest, "#")
	if frag != "" {
		frag = "#" + frag
	}
	repoPath := path.Clean(path.Join("docs", path.Dir(fromRel), target))
	if rel, ok := strings.CutPrefix(repoPath, "docs/"); ok {
		if p, ok := b.pages[rel]; ok {
			return p.URL + frag
		}
		if p, ok := b.pages[path.Join(rel, "README.md")]; ok {
			return p.URL + frag
		}
	}
	if repoPath == "docs" {
		return "/docs/" + frag
	}
	return repoBlob + repoPath + frag
}

func (b *builder) write(p *page, nav []navSection) error {
	var buf bytes.Buffer
	err := pageTemplate.Execute(&buf, map[string]any{
		"Page":     p,
		"Nav":      nav,
		"Base":     b.base,
		"EditURL":  repoBlob + "docs/" + p.Rel,
		"EmailOff": template.HTML("<!--email_off-->"),
		"EmailOn":  template.HTML("<!--/email_off-->"),
	})
	if err != nil {
		return err
	}
	dst := filepath.Join(b.out, filepath.FromSlash(p.out))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, buf.Bytes(), 0o644)
}

func (b *builder) sitemap() error {
	type entry struct {
		Loc string `xml:"loc"`
	}
	urls := make([]string, 0, len(b.pages))
	for _, p := range b.pages {
		urls = append(urls, b.base+p.URL)
	}
	sort.Strings(urls)
	set := struct {
		XMLName xml.Name `xml:"urlset"`
		NS      string   `xml:"xmlns,attr"`
		URLs    []entry  `xml:"url"`
	}{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, u := range urls {
		set.URLs = append(set.URLs, entry{Loc: u})
	}
	data, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(b.out, "sitemap.xml"), append([]byte(xml.Header), append(data, '\n')...), 0o644)
}
