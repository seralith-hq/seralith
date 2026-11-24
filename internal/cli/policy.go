package cli

import (
	"fmt"
	"os"

	"github.com/ryan-tobin/new-startup/internal/policy"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/spf13/cobra"
)

func newPolicyCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{Use: "policy", Short: "Inspect and validate policy"}
	cmd.AddCommand(&cobra.Command{
		Use:   "lint [file]",
		Short: "Validate a policy file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			} else {
				p, err := project.Load(cmd.Context(), g.dir)
				if err != nil {
					return err
				}
				path = p.ConfigPath(project.PolicyFile)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			p, err := policy.Parse(data)
			if err != nil {
				return &ExitError{Code: 1, Err: err}
			}
			fmt.Fprintf(g.stdout, "%s %s: %s, sha256 %s\n", styleFor(g.stdout).green("✓"), path, plural(len(p.Rules), "rule"), short(p.Digest()))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "List the rules in effect",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, e.Policy)
			}
			s := styleFor(g.stdout)
			rows := make([][]string, 0, len(e.Policy.Rules))
			for _, r := range e.Policy.Rules {
				rows = append(rows, []string{r.ID, s.status(string(r.Effect)), r.Description})
			}
			table(g.stdout, []string{"RULE", "EFFECT", "DESCRIPTION"}, rows)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "default",
		Short: "Print the default policy",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			fmt.Fprint(g.stdout, policy.Default)
			return nil
		},
	})
	return cmd
}
