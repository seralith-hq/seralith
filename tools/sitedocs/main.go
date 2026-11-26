// Command sitedocs renders the repository's docs/ folder into the /docs section of
// seralith.com.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	src := flag.String("src", "docs", "directory of Markdown documentation")
	out := flag.String("out", "site/docs", "directory to write the generated pages to; it is replaced")
	base := flag.String("base", "https://seralith.com", "public origin of the site")
	flag.Parse()
	if err := Build(*src, *out, *base); err != nil {
		fmt.Fprintln(os.Stderr, "sitedocs:", err)
		os.Exit(1)
	}
}
