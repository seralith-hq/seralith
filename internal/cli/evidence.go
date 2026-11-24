package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/ryan-tobin/new-startup/internal/evidence"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/ryan-tobin/new-startup/internal/verify"
	"github.com/spf13/cobra"
)

func newEvidenceCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{Use: "evidence", Short: "Export and verify signed evidence packages for assessors"}

	var rangeSpec, out string
	export := &cobra.Command{
		Use:   "export --range <a..b>",
		Short: "Write a signed evidence package (zip) for a revision range",
		Long: `Write a signed evidence package for a revision range.

The package contains every attestation in the range, the gate report, the
policy and trust store in effect, the audit ledger and its verification,
a record of coding tools and declared models, a control mapping for
DoWI 8430.01, NIST SP 800-218, and the EU CRA, and a signed manifest.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if rangeSpec == "" {
				return fmt.Errorf("--range is required")
			}
			gate, err := verify.Gate(cmd.Context(), e.Project.Repo, rangeSpec, verify.Options{Trust: e.Trust, Policy: e.Policy, SkipMerges: true})
			if err != nil {
				return err
			}
			policyBytes, _ := os.ReadFile(e.Project.ConfigPath(project.PolicyFile))
			trustBytes, _ := os.ReadFile(e.Project.ConfigPath(project.TrustFile))
			if out == "" {
				out = fmt.Sprintf("sigil-evidence-%s.zip", time.Now().UTC().Format("20060102T150405Z"))
			}
			f, err := os.Create(out)
			if err != nil {
				return err
			}
			man, err := evidence.Build(cmd.Context(), evidence.Input{
				Repo: e.Project.Repo, Range: rangeSpec, Gate: gate, PolicyFile: policyBytes, TrustFile: trustBytes,
				Ledger: e.Ledger, Signer: e.Key, Project: e.Project.Config.Name, Now: time.Now().UTC(),
			}, f)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			_, _ = e.Ledger.Append("evidence.exported", "human:"+e.Principal, rangeSpec, map[string]any{"file": out, "files": len(man.Files), "status": man.Status})
			if g.json {
				return writeJSON(g.stdout, map[string]any{"file": out, "manifest": man})
			}
			s := styleFor(g.stdout)
			fmt.Fprintf(g.stdout, "%s wrote %s (%s, gate %s)\n", s.green("✓"), out, plural(len(man.Files)+2, "file"), s.status(string(man.Status)))
			return nil
		},
	}
	export.Flags().StringVar(&rangeSpec, "range", "", "revision range, e.g. v1.0.0..v1.1.0")
	export.Flags().StringVarP(&out, "out", "o", "", "output file")

	var trustPath string
	verifyCmd := &cobra.Command{
		Use:   "verify <package.zip>",
		Short: "Verify an evidence package's manifest hashes and signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var trust *keys.TrustStore
			var err error
			if trustPath != "" {
				trust, err = keys.LoadTrustStore(trustPath)
			} else if e, lerr := loadEnv(cmd.Context(), g); lerr == nil {
				trust = e.Trust
			} else {
				err = fmt.Errorf("pass --trust when not inside a Sigil project")
			}
			if err != nil {
				return err
			}
			res, err := evidence.Verify(args[0], trust)
			if err != nil {
				return err
			}
			if g.json {
				_ = writeJSON(g.stdout, res)
			} else {
				s := styleFor(g.stdout)
				state := s.green("verified")
				if !res.OK() {
					state = s.red("invalid")
				}
				fmt.Fprintf(g.stdout, "%s  %s range=%s files=%d signed=%v\n", state, args[0], res.Manifest.Range, len(res.Manifest.Files), res.Signed)
				for _, m := range res.Mismatch {
					fmt.Fprintf(g.stdout, "  %s hash mismatch: %s\n", s.red("✗"), m)
				}
				for _, m := range res.Missing {
					fmt.Fprintf(g.stdout, "  %s missing: %s\n", s.red("✗"), m)
				}
			}
			if !res.OK() {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	verifyCmd.Flags().StringVar(&trustPath, "trust", "", "trust store to verify the manifest signature against")
	cmd.AddCommand(export, verifyCmd)
	return cmd
}
