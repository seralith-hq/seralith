package main

import (
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const blogIndexDesc = "Notes on how the Seralith gate, attestations, and policy are built."

type frontMatter struct {
	Title   string `yaml:"title"`
	Date    string `yaml:"date"`
	Summary string `yaml:"summary"`
}

// BuildBlog renders every Markdown post under src into HTML pages under out,
// plus an index. README.md is the folder's own note and is not a post.
func BuildBlog(src, out, base string) error {
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
	var posts []*page
	err := walkMarkdown(src, func(rel string, d fs.DirEntry) bool {
		return !d.IsDir() && path.Base(rel) == "README.md"
	}, func(rel string, data []byte) error {
		pg := &page{Rel: rel}
		body, err := parsePost(rel, data, pg)
		if err != nil {
			return err
		}
		pg.source = body
		pg.URL, pg.out = locateBlog(rel)
		b.pages[rel] = pg
		posts = append(posts, pg)
		return nil
	})
	if err != nil {
		return err
	}
	if len(posts) == 0 {
		return errors.New("no blog posts found")
	}
	sort.Slice(posts, func(i, j int) bool {
		if !posts[i].when.Equal(posts[j].when) {
			return posts[i].when.After(posts[j].when)
		}
		return posts[i].Title < posts[j].Title
	})
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	for _, p := range posts {
		html, err := renderMarkdown(b.md, p.source, func(dest string) string {
			return b.rewriteBlog(p.Rel, dest)
		})
		if err != nil {
			return fmt.Errorf("%s: %w", p.Rel, err)
		}
		p.HTML = template.HTML(html) //nolint:gosec // goldmark escapes raw HTML because the unsafe option is off
		if err := b.writeBlog(p, posts, false); err != nil {
			return fmt.Errorf("%s: %w", p.Rel, err)
		}
	}
	index := &page{
		URL:   "/blog/",
		Title: "Blog",
		Desc:  blogIndexDesc,
		out:   "index.html",
	}
	if err := b.writeBlog(index, posts, true); err != nil {
		return err
	}
	b.pages["index"] = index
	return b.sitemap()
}

func locateBlog(rel string) (urlPath, file string) {
	slug := strings.TrimSuffix(rel, ".md")
	return "/blog/" + slug, slug + ".html"
}

func parsePost(rel string, src []byte, pg *page) ([]byte, error) {
	fm, body, err := splitFrontMatter(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	fm.Title = strings.TrimSpace(fm.Title)
	fm.Summary = strings.Join(strings.Fields(fm.Summary), " ")
	if fm.Title == "" || fm.Summary == "" || strings.TrimSpace(fm.Date) == "" {
		return nil, fmt.Errorf("%s: front matter needs title, date, and summary", rel)
	}
	when, err := parseDate(strings.TrimSpace(fm.Date))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	pg.Title = fm.Title
	pg.Summary = fm.Summary
	pg.Desc = fm.Summary
	pg.when = when
	pg.Date = when.Format(time.DateOnly)
	pg.DateLabel = when.Format("2 January 2006")
	return body, nil
}

func splitFrontMatter(src []byte) (frontMatter, []byte, error) {
	s := string(src)
	nl := "\n"
	switch {
	case strings.HasPrefix(s, "---\r\n"):
		nl = "\r\n"
	case strings.HasPrefix(s, "---\n"):
	default:
		return frontMatter{}, nil, errors.New("missing front matter")
	}
	rest := s[len("---"+nl):]
	closer := nl + "---" + nl
	i := strings.Index(rest, closer)
	var block, body string
	if i >= 0 {
		block, body = rest[:i], rest[i+len(closer):]
	} else if strings.HasSuffix(rest, nl+"---") {
		block = strings.TrimSuffix(rest, nl+"---")
	} else {
		return frontMatter{}, nil, errors.New("front matter is not closed")
	}
	var fm frontMatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return frontMatter{}, nil, err
	}
	return fm, []byte(body), nil
}

func parseDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	return time.Time{}, fmt.Errorf("date must be YYYY-MM-DD, got %q", s)
}

// rewriteBlog maps a link in a post to another post, or to the file on GitHub.
func (b *builder) rewriteBlog(fromRel, dest string) string {
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
	rel := path.Clean(path.Join(path.Dir(fromRel), target))
	if p, ok := b.pages[rel]; ok {
		return p.URL + frag
	}
	if !strings.HasSuffix(rel, ".md") {
		if p, ok := b.pages[rel+".md"]; ok {
			return p.URL + frag
		}
	}
	repoPath := path.Clean(path.Join("blog", path.Dir(fromRel), target))
	return repoBlob + repoPath + frag
}

func (b *builder) writeBlog(p *page, posts []*page, index bool) error {
	var buf strings.Builder
	err := blogTemplate.Execute(&buf, map[string]any{
		"Page":     p,
		"Posts":    posts,
		"Index":    index,
		"Base":     b.base,
		"EditURL":  repoBlob + "blog/" + p.Rel,
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
	return os.WriteFile(dst, []byte(buf.String()), 0o644)
}
