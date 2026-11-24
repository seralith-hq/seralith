package cli

import (
	"fmt"
	"strings"

	"github.com/ryan-tobin/new-startup/internal/workspace"
	"github.com/spf13/cobra"
)

func newWorkspaceCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ws",
		Aliases: []string{"workspace"},
		Short:   "Create and manage isolated workspaces",
	}

	var base string
	newCmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a workspace on a new sigil/<name> branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if err := e.requireIdentity(); err != nil {
				return err
			}
			ws, err := e.Workspaces.Create(cmd.Context(), workspace.CreateOptions{Name: args[0], Base: base, Owner: e.Principal, Log: g.stderr})
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, ws)
			}
			s := styleFor(g.stdout)
			fmt.Fprintf(g.stdout, "%s workspace %s on %s from %s\n  %s\n", s.green("✓"), s.bold(ws.Name), ws.Branch, short(ws.BaseCommit), ws.Path)
			return nil
		},
	}
	newCmd.Flags().StringVar(&base, "base", "HEAD", "commit or branch to start from")

	lsCmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List workspaces",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			list, err := e.Workspaces.List()
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, list)
			}
			if len(list) == 0 {
				fmt.Fprintln(g.stdout, "no workspaces; create one with: sigil ws new <name>")
				return nil
			}
			s := styleFor(g.stdout)
			rows := make([][]string, 0, len(list))
			for _, w := range list {
				pending := s.dim("clean")
				if w.Pending() {
					pending = s.yellow(plural(len(w.Segments), "segment"))
				}
				rows = append(rows, []string{w.Name, w.Branch, short(w.Head), pending, w.UpdatedAt.Local().Format("2006-01-02 15:04"), w.Path})
			}
			table(g.stdout, []string{"NAME", "BRANCH", "HEAD", "PENDING", "UPDATED", "PATH"}, rows)
			return nil
		},
	}

	pathCmd := &cobra.Command{
		Use:   "path <name>",
		Short: "Print a workspace's directory, for use with cd \"$(sigil ws path x)\"",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			ws, err := e.Workspaces.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(g.stdout, ws.Path)
			return nil
		},
	}

	var force, keepBranch bool
	rmCmd := &cobra.Command{
		Use:     "rm <name>",
		Aliases: []string{"remove"},
		Short:   "Remove a workspace and its branch",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if err := e.Workspaces.Remove(cmd.Context(), args[0], workspace.RemoveOptions{Force: force, DeleteBranch: !keepBranch, Actor: "human:" + e.Principal}); err != nil {
				return err
			}
			fmt.Fprintf(g.stdout, "removed workspace %s\n", args[0])
			return nil
		},
	}
	rmCmd.Flags().BoolVar(&force, "force", false, "discard unattested segments and uncommitted changes")
	rmCmd.Flags().BoolVar(&keepBranch, "keep-branch", false, "keep the sigil/<name> branch")

	cmd.AddCommand(newCmd, lsCmd, pathCmd, rmCmd)
	return cmd
}

func joinCommand(c []string) string {
	parts := make([]string, len(c))
	for i, p := range c {
		if strings.ContainsAny(p, " \t\"'") {
			parts[i] = fmt.Sprintf("%q", p)
		} else {
			parts[i] = p
		}
	}
	return strings.Join(parts, " ")
}
