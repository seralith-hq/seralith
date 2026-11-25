package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ryan-tobin/new-startup/internal/notes"
	"github.com/ryan-tobin/new-startup/internal/verify"
	"github.com/spf13/cobra"
)

func newVerifyCmd(g *globals) *cobra.Command {
	var configFrom string
	cmd := &cobra.Command{
		Use:   "verify [commit]",
		Short: "Verify a commit's attestation, signatures, integrity, and policy",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if err := e.useConfigFrom(cmd.Context(), configFrom); err != nil {
				return err
			}
			rev := "HEAD"
			if len(args) == 1 {
				rev = args[0]
			}
			repo := e.Project.Repo
			if ws, err := e.Workspaces.ForPath(g.dir); err == nil {
				repo = ws.Repo()
			}
			rep, err := verify.Commit(cmd.Context(), repo, rev, verify.Options{Trust: e.Trust, Policy: e.Policy})
			if err != nil {
				return err
			}
			if g.json {
				if err := writeJSON(g.stdout, rep); err != nil {
					return err
				}
			} else {
				printCommitReport(g.stdout, rep, true)
			}
			if rep.Status == verify.StatusFail {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&configFrom, "config-from", "", "load policy and trust store from this revision instead of the working tree")
	return cmd
}

func newGateCmd(g *globals) *cobra.Command {
	var rangeSpec, base, format, remote, configFrom string
	var fetch, includeMerges bool
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Verify every commit in a range; exits non-zero on failure (for CI)",
		Example: `  seralith gate --base origin/main
  seralith gate --range v1.2.0..v1.3.0 --format markdown
  seralith gate --base origin/main --fetch --format github`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			repo := e.Project.Repo
			if err := e.useConfigFrom(cmd.Context(), configFrom); err != nil {
				return err
			}
			if fetch {
				if err := notes.Fetch(cmd.Context(), repo, remote); err != nil {
					fmt.Fprintf(g.stderr, "warning: could not fetch attestations from %s: %v\n", remote, err)
				}
			}
			if rangeSpec == "" {
				rangeSpec = base + "..HEAD"
			}
			rep, err := verify.Gate(cmd.Context(), repo, rangeSpec, verify.Options{Trust: e.Trust, Policy: e.Policy, SkipMerges: !includeMerges})
			if err != nil {
				return err
			}
			_, _ = e.Ledger.Append("gate.run", "human:"+e.Principal, rangeSpec, map[string]any{"status": rep.Status, "totals": rep.Totals})
			if g.json {
				format = "json"
			}
			switch format {
			case "json":
				if err := writeJSON(g.stdout, rep); err != nil {
					return err
				}
			case "markdown":
				fmt.Fprint(g.stdout, gateMarkdown(rep))
			case "github":
				for _, c := range rep.Commits {
					for _, p := range c.Problems {
						fmt.Fprintf(g.stdout, "::error title=Seralith gate (%s)::%s\n", short(c.Commit), p)
					}
					for _, w := range c.Warnings {
						fmt.Fprintf(g.stdout, "::warning title=Seralith gate (%s)::%s\n", short(c.Commit), w)
					}
				}
				if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
					if f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600); err == nil {
						_, _ = io.WriteString(f, gateMarkdown(rep))
						_ = f.Close()
					}
				}
				printGate(g.stdout, rep)
			default:
				printGate(g.stdout, rep)
			}
			if rep.Status == verify.StatusFail {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&rangeSpec, "range", "", "revision range to verify, e.g. origin/main..HEAD")
	cmd.Flags().StringVar(&base, "base", "origin/main", "base revision when --range is not given")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, markdown, github")
	cmd.Flags().BoolVar(&fetch, "fetch", false, "fetch attestations from the remote first")
	cmd.Flags().StringVar(&remote, "remote", "origin", "remote to fetch attestations from")
	cmd.Flags().BoolVar(&includeMerges, "include-merges", false, "require attestations on merge commits too")
	cmd.Flags().StringVar(&configFrom, "config-from", "", "load policy and trust store from this revision (use the base branch in CI)")
	return cmd
}

func printGate(w io.Writer, rep *verify.GateReport) {
	s := styleFor(w)
	fmt.Fprintf(w, "%s %s  %s\n\n", s.bold("seralith gate"), rep.Range, s.status(string(rep.Status)))
	if len(rep.Commits) == 0 {
		fmt.Fprintln(w, "no commits in range")
		return
	}
	for _, c := range rep.Commits {
		printCommitReport(w, c, false)
	}
	fmt.Fprintf(w, "\n%d pass · %d warn · %d fail · %d skipped   lines: %s\n",
		rep.Totals[verify.StatusPass], rep.Totals[verify.StatusWarn], rep.Totals[verify.StatusFail], rep.Totals[verify.StatusSkipped], formatLineMap(rep.Lines))
}

func printCommitReport(w io.Writer, c *verify.CommitReport, verbose bool) {
	s := styleFor(w)
	fmt.Fprintf(w, "%-6s %s %s\n", s.status(string(c.Status)), s.bold(short(c.Commit)), c.Subject)
	if verbose {
		if len(c.Signers) > 0 {
			fmt.Fprintf(w, "       signed by %s\n", strings.Join(c.Signers, ", "))
		}
		if c.Summary != nil {
			fmt.Fprintf(w, "       attribution %s\n", formatLines(c.Summary.LinesByKind))
		}
		for _, d := range c.Decisions {
			state := s.green("satisfied")
			if !d.Satisfied {
				state = s.red("unsatisfied")
			}
			fmt.Fprintf(w, "       %s %s %s %s\n", s.status(string(d.Effect)), d.Rule, state, s.dim(strings.Join(d.Approvers, ",")))
		}
	}
	for _, p := range c.Problems {
		fmt.Fprintf(w, "       %s %s\n", s.red("✗"), p)
	}
	for _, warn := range c.Warnings {
		fmt.Fprintf(w, "       %s %s\n", s.yellow("!"), warn)
	}
}

func formatLineMap(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	parts := []string{}
	for _, k := range []string{"human", "agent"} {
		if v, ok := m[k]; ok {
			parts = append(parts, fmt.Sprintf("%s %d", k, v))
		}
	}
	return strings.Join(parts, " · ")
}

func gateMarkdown(rep *verify.GateReport) string {
	var b strings.Builder
	icon := map[verify.Status]string{verify.StatusPass: "✅", verify.StatusWarn: "⚠️", verify.StatusFail: "❌", verify.StatusSkipped: "⏭️"}
	fmt.Fprintf(&b, "### %s Seralith gate: %s\n\n", icon[rep.Status], strings.ToUpper(string(rep.Status)))
	fmt.Fprintf(&b, "Range `%s` · human lines %d · agent lines %d\n\n", rep.Range, rep.Lines["human"], rep.Lines["agent"])
	fmt.Fprintf(&b, "| | Commit | Subject | Details |\n| --- | --- | --- | --- |\n")
	for _, c := range rep.Commits {
		details := append(append([]string{}, c.Problems...), c.Warnings...)
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s |\n", icon[c.Status], short(c.Commit), strings.ReplaceAll(c.Subject, "|", "\\|"), strings.ReplaceAll(strings.Join(details, "<br>"), "|", "\\|"))
	}
	return b.String()
}
