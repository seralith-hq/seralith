package cli

import (
	"fmt"

	"github.com/seralith-hq/seralith/internal/ledger"
	"github.com/spf13/cobra"
)

func newLogCmd(g *globals) *cobra.Command {
	var typ, subject string
	var limit int
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show the audit ledger",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			entries, err := e.Ledger.Entries(ledger.Filter{Type: typ, Subject: subject, Limit: limit})
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, entries)
			}
			s := styleFor(g.stdout)
			for _, en := range entries {
				fmt.Fprintf(g.stdout, "%s %s %-22s %-28s %s\n", s.dim(fmt.Sprintf("%5d", en.Seq)), en.Time.Local().Format("2006-01-02 15:04:05"), s.accent(en.Type), truncate(en.Subject, 28), s.dim(truncate(en.Actor, 48)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "filter by entry type, e.g. commit.attested")
	cmd.Flags().StringVar(&subject, "subject", "", "filter by subject")
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "show the most recent n entries (0 for all)")
	return cmd
}

func newLedgerCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{Use: "ledger", Short: "Verify and checkpoint the audit ledger"}
	var strict bool
	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the hash chain and checkpoint signatures",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			rep, err := e.Ledger.Verify(e.Trust)
			if err != nil {
				return err
			}
			if g.json {
				_ = writeJSON(g.stdout, rep)
			} else {
				s := styleFor(g.stdout)
				state := s.green("intact")
				switch {
				case !rep.OK():
					state = s.red("tampered")
				case !rep.Anchored():
					state = s.yellow("unanchored")
				}
				fmt.Fprintf(g.stdout, "%s  %d entries, %d/%d checkpoints verified, head %s\n", state, rep.Entries, rep.Verified, rep.Checkpoints, short(rep.Head))
				for _, is := range rep.Issues {
					fmt.Fprintf(g.stdout, "  %s entry %d: %s\n", s.red("✗"), is.Seq, is.Problem)
				}
				if rep.OK() && !rep.Anchored() {
					fmt.Fprintf(g.stdout, "  %s the hash chain is intact, but without checkpoints signed by trusted keys it does not prove the ledger was not rewritten\n", s.yellow("!"))
					for _, id := range rep.UntrustedKeys {
						fmt.Fprintf(g.stdout, "  %s checkpoint key %s is not in the trust store\n", s.yellow("!"), id)
					}
				}
			}
			if !rep.OK() || (strict && !rep.Anchored()) {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	verifyCmd.Flags().BoolVar(&strict, "strict", false, "also fail unless every checkpoint is signed by a trusted key")
	cmd.AddCommand(verifyCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "checkpoint",
		Short: "Sign the current ledger head",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			en, err := e.Ledger.Checkpoint(e.Key)
			if err != nil {
				return err
			}
			fmt.Fprintf(g.stdout, "checkpoint %d signed by %s over %s\n", en.Seq, e.Key.KeyID(), short(en.Subject))
			return nil
		},
	})
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
