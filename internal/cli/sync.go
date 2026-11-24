package cli

import (
	"fmt"

	"github.com/ryan-tobin/new-startup/internal/notes"
	"github.com/spf13/cobra"
)

func newSyncCmd(g *globals) *cobra.Command {
	var remote string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Push or fetch attestations (refs/notes/sigil)",
	}
	cmd.PersistentFlags().StringVar(&remote, "remote", "origin", "git remote")
	cmd.AddCommand(&cobra.Command{
		Use:   "push",
		Short: "Publish local attestations to the remote",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if err := notes.Push(cmd.Context(), e.Project.Repo, remote); err != nil {
				return err
			}
			fmt.Fprintf(g.stdout, "pushed %s to %s\n", notes.Ref, remote)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "fetch",
		Short: "Fetch and merge attestations from the remote",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if err := notes.Fetch(cmd.Context(), e.Project.Repo, remote); err != nil {
				return err
			}
			fmt.Fprintf(g.stdout, "fetched %s from %s\n", notes.Ref, remote)
			return nil
		},
	})
	return cmd
}
