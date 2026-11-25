package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"
)

type style struct{ enabled bool }

func styleFor(w io.Writer) style {
	f, ok := w.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return style{}
	}
	return style{enabled: term.IsTerminal(int(f.Fd()))}
}

func (s style) wrap(code, v string) string {
	if !s.enabled {
		return v
	}
	return "\x1b[" + code + "m" + v + "\x1b[0m"
}

func (s style) bold(v string) string   { return s.wrap("1", v) }
func (s style) dim(v string) string    { return s.wrap("2", v) }
func (s style) red(v string) string    { return s.wrap("31", v) }
func (s style) green(v string) string  { return s.wrap("32", v) }
func (s style) yellow(v string) string { return s.wrap("33", v) }
func (s style) accent(v string) string { return s.wrap("38;5;202", v) }

func (s style) status(v string) string {
	switch strings.ToLower(v) {
	case "pass", "allow", "approve", "running", "active", "ok", "verified":
		return s.green(v)
	case "warn", "require-approval", "pending":
		return s.yellow(v)
	case "fail", "deny", "reject", "failed":
		return s.red(v)
	}
	return v
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func table(w io.Writer, header []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	_ = tw.Flush()
}

// shortID shortens a generated ID such as ses_<time><random> to its prefix and the
// end of its random part, which, unlike the leading timestamp, tells IDs created
// close together apart.
func shortID(id string) string {
	prefix, rest, ok := strings.Cut(id, "_")
	if !ok || len(rest) <= 8 {
		return id
	}
	return prefix + "_" + rest[len(rest)-8:]
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
