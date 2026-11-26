// Command sitedocs renders the repository's docs/ folder into the /docs section
// of seralith.com, and the blog/ folder into /blog.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	src := flag.String("src", "docs", "directory of Markdown documentation")
	out := flag.String("out", "site/docs", "directory to write the generated docs pages to; it is replaced")
	blog := flag.String("blog", "blog", "directory of Markdown posts")
	blogOut := flag.String("blog-out", "site/blog", "directory to write the generated blog pages to; it is replaced")
	base := flag.String("base", "https://seralith.com", "public origin of the site")
	flag.Parse()
	if err := Build(*src, *out, *base); err != nil {
		fmt.Fprintln(os.Stderr, "sitedocs:", err)
		os.Exit(1)
	}
	if err := BuildBlog(*blog, *blogOut, *base); err != nil {
		fmt.Fprintln(os.Stderr, "sitedocs:", err)
		os.Exit(1)
	}
}
