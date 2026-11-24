package cli

import (
	"fmt"
	"strings"

	"github.com/ryan-tobin/new-startup/internal/blame"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
	"github.com/spf13/cobra"
)

func newBlameCmd(g *globals) *cobra.Command {
	var rev string
	var summaryOnly bool
	cmd := &cobra.Command{
		Use:   "blame <file>",
		Short: "Show which human or agent authored each line of a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			repo := e.Project.Repo
			if ws, err := e.Workspaces.ForPath(g.dir); err == nil {
				repo = ws.Repo()
			}
			lines, sum, err := blame.File(cmd.Context(), repo, rev, args[0], e.Trust)
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, map[string]any{"lines": lines, "summary": sum})
			}
			s := styleFor(g.stdout)
			if !summaryOnly {
				for _, l := range lines {
					who := s.dim("unattested")
					if l.Attested {
						who = l.Actor.ID
						if l.Actor.Kind == provenance.ActorAgent {
							who = s.accent(who)
						}
						if !l.Verified {
							who += s.red("?")
						}
					}
					fmt.Fprintf(g.stdout, "%s %-24s %4d │ %s\n", s.dim(short(l.Commit)[:8]), pad(who, l.Actor.ID, 24), l.Number, l.Text)
				}
				fmt.Fprintln(g.stdout)
			}
			parts := []string{fmt.Sprintf("%d lines", sum.Lines)}
			for _, k := range []provenance.ActorKind{provenance.ActorHuman, provenance.ActorAgent} {
				if v := sum.ByKind[k]; v > 0 {
					parts = append(parts, fmt.Sprintf("%s %d (%d%%)", k, v, v*100/max(sum.Lines, 1)))
				}
			}
			if sum.Unattested > 0 {
				parts = append(parts, fmt.Sprintf("unattested %d", sum.Unattested))
			}
			fmt.Fprintln(g.stdout, strings.Join(parts, " · "))
			return nil
		},
	}
	cmd.Flags().StringVar(&rev, "rev", "HEAD", "revision to blame")
	cmd.Flags().BoolVar(&summaryOnly, "summary", false, "print only the summary")
	return cmd
}

func pad(styled, plain string, width int) string {
	if n := width - len(plain); n > 0 && len(styled) > len(plain) {
		return styled + strings.Repeat(" ", n)
	}
	return styled
}
