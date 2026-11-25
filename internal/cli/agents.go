package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAgentsCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "List coding tools Seralith can launch and whether they are installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			det := e.Agents.Detect()
			if g.json {
				return writeJSON(g.stdout, det)
			}
			s := styleFor(g.stdout)
			rows := make([][]string, 0, len(det))
			for _, d := range det {
				state := s.dim("not found")
				if d.Installed {
					state = s.green("installed")
				}
				rows = append(rows, []string{d.Profile.ID, d.Profile.Name, string(d.Profile.Kind), state, d.Path})
			}
			table(g.stdout, []string{"ID", "NAME", "KIND", "STATUS", "PATH"}, rows)
			fmt.Fprintln(g.stdout, s.dim("\nSeralith launches these tools as-is. It never proxies model traffic or handles your credentials."))
			return nil
		},
	}
}
