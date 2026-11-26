package main

import "html/template"

const navTemplate = `{{define "nav"}}
        {{- range .Nav}}
        <p class="docs-nav-title">{{.Name}}</p>
        <ul>
          {{- range .Pages}}
          <li><a href="{{.URL}}"{{if eq .URL $.Page.URL}} aria-current="page"{{end}}>{{.NavTitle}}</a></li>
          {{- end}}
        </ul>
        {{- end}}
{{- end}}`

var pageTemplate = template.Must(template.Must(template.New("page").Parse(navTemplate)).Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>{{.Page.Title}} · Seralith docs</title>
    {{- with .Page.Desc}}
    <meta name="description" content="{{.}}" />
    <meta property="og:description" content="{{.}}" />
    {{- end}}
    <meta property="og:title" content="{{.Page.Title}} · Seralith docs" />
    <meta property="og:url" content="{{.Base}}{{.Page.URL}}" />
    <meta property="og:image" content="{{.Base}}/og-image.png" />
    <meta name="twitter:card" content="summary_large_image" />
    <meta name="twitter:site" content="@seralithhq" />
    <meta name="theme-color" content="#0E1116" />
    <link rel="canonical" href="{{.Base}}{{.Page.URL}}" />
    <link rel="icon" href="/favicon.ico" sizes="any" />
    <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
    <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
    <link rel="stylesheet" href="/styles.css" />
  </head>
  <body>
  {{.EmailOff}}
    <header class="nav">
      <div class="wrap nav-inner">
        <a class="logo" href="/" aria-label="Seralith home">
          <img src="/logo.svg" alt="Seralith" height="28" />
        </a>
        <nav aria-label="Primary">
          <a href="/docs/">Docs</a>
          <a href="https://github.com/seralith-hq/seralith">GitHub</a>
          <a href="/security">Security</a>
          <a class="btn btn-small" href="/#contact">Book a pilot</a>
        </nav>
      </div>
    </header>

    <div class="wrap docs">
      <nav class="docs-nav" aria-label="Documentation">
        {{- template "nav" .}}
      </nav>
      <details class="docs-menu">
        <summary>Documentation</summary>
        <nav aria-label="Documentation">
          {{- template "nav" .}}
        </nav>
      </details>
      <main class="doc">
{{.Page.HTML}}
        <p class="doc-edit small"><a href="{{.EditURL}}">Edit this page on GitHub</a></p>
      </main>
    </div>

    <footer class="wrap footer">
      <span>© 2026 Seralith, Inc.</span>
      <span><a class="muted" href="/security">Security</a> · <a class="muted" href="https://github.com/seralith-hq/seralith" rel="noopener">GitHub</a> · <a class="muted" href="https://x.com/seralithhq" rel="noopener me">X</a></span>
    </footer>
  {{.EmailOn}}
  </body>
</html>
`))
