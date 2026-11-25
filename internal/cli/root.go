// Package cli implements the seralith command-line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/ryan-tobin/new-startup/internal/version"
	"github.com/spf13/cobra"
)

// ExitError carries a specific process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

type globals struct {
	dir    string
	json   bool
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// Main runs the CLI and returns the process exit code.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	g := &globals{stdin: stdin, stdout: stdout, stderr: stderr}
	root := newRoot(g)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		if exit.Err != nil {
			fmt.Fprintln(stderr, styleFor(stderr).red("error:"), exit.Err)
		}
		return exit.Code
	}
	fmt.Fprintln(stderr, styleFor(stderr).red("error:"), err)
	return 1
}

func newRoot(g *globals) *cobra.Command {
	root := &cobra.Command{
		Use:           "seralith",
		Short:         "Build in parallel. Ship with proof.",
		Long:          "Seralith runs human and agent coding sessions in isolated workspaces and\nrecords signed, line-level provenance for every change.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.String(),
	}
	root.SetVersionTemplate("seralith {{.Version}}\n")
	root.PersistentFlags().StringVarP(&g.dir, "dir", "C", ".", "run as if seralith was started in this directory")
	root.PersistentFlags().BoolVar(&g.json, "json", false, "emit machine-readable JSON")

	root.AddGroup(
		&cobra.Group{ID: "setup", Title: "Setup:"},
		&cobra.Group{ID: "work", Title: "Workspaces and sessions:"},
		&cobra.Group{ID: "trust", Title: "Provenance and verification:"},
		&cobra.Group{ID: "ops", Title: "Operations:"},
	)
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("setup", newInitCmd(g), newWhoamiCmd(g), newKeyCmd(g), newAgentsCmd(g))
	add("work", newWorkspaceCmd(g), newRunCmd(g), newStatusCmd(g), newCommitCmd(g))
	add("trust", newApproveCmd(g), newVerifyCmd(g), newGateCmd(g), newBlameCmd(g), newEvidenceCmd(g), newPolicyCmd(g))
	add("ops", newLogCmd(g), newLedgerCmd(g), newSyncCmd(g), newServeCmd(g), newCloudCmd(g), newVersionCmd(g))
	return root
}

func newVersionCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if g.json {
				return writeJSON(g.stdout, map[string]string{"version": version.Version, "commit": version.Commit, "date": version.Date})
			}
			fmt.Fprintln(g.stdout, "seralith", version.String())
			return nil
		},
	}
}
