package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/spf13/cobra"
)

func newWhoamiCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show your identity, signing key, and trust status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			tk, trusted := e.Trust.Lookup(e.Key.KeyID())
			out := map[string]any{"principal": e.Principal, "key": e.Key.KeyID(), "keyPath": project.UserKeyPath(), "trusted": trusted && !tk.Revoked}
			if trusted {
				out["roles"] = tk.Roles
				out["revoked"] = tk.Revoked
			}
			if g.json {
				return writeJSON(g.stdout, out)
			}
			s := styleFor(g.stdout)
			fmt.Fprintf(g.stdout, "principal  %s\nkey        %s\nkey file   %s\n", e.Principal, e.Key.KeyID(), project.UserKeyPath())
			switch {
			case !trusted:
				fmt.Fprintf(g.stdout, "trust      %s (ask a maintainer to run: seralith key trust <your public key> --principal %s)\n", s.red("not trusted"), e.Principal)
			case tk.Revoked:
				fmt.Fprintf(g.stdout, "trust      %s\n", s.red("revoked"))
			default:
				fmt.Fprintf(g.stdout, "trust      %s roles=%s\n", s.green("trusted"), strings.Join(tk.Roles, ","))
			}
			return nil
		},
	}
}

func newKeyCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{Use: "key", Short: "Manage signing keys and the trust store"}

	cmd.AddCommand(&cobra.Command{
		Use:   "export",
		Short: "Print your public key in PEM format",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			k, _, err := keys.LoadOrCreate(project.UserKeyPath())
			if err != nil {
				return err
			}
			pemText, err := k.Public().PEM()
			if err != nil {
				return err
			}
			fmt.Fprint(g.stdout, pemText)
			return nil
		},
	})

	var principal, kind string
	var roles []string
	trustCmd := &cobra.Command{
		Use:   "trust <public-key.pem>",
		Short: "Add a public key to the repository trust store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			pub, err := keys.ParsePublicPEM(data)
			if err != nil {
				return err
			}
			if principal == "" {
				return fmt.Errorf("--principal is required")
			}
			k := keys.PrincipalKind(kind)
			if k != keys.KindHuman && k != keys.KindMachine {
				return fmt.Errorf("--kind must be human or machine")
			}
			tk, err := e.Trust.Add(pub, principal, k, roles...)
			if err != nil {
				return err
			}
			if err := e.Trust.Save(e.Project.ConfigPath(project.TrustFile)); err != nil {
				return err
			}
			_, _ = e.Ledger.Append("key.trusted", "human:"+e.Principal, tk.ID, map[string]any{"principal": principal, "kind": kind, "roles": tk.Roles})
			fmt.Fprintf(g.stdout, "trusted %s for %s (%s) roles=%s\n", tk.ID, principal, kind, strings.Join(tk.Roles, ","))
			return nil
		},
	}
	trustCmd.Flags().StringVar(&principal, "principal", "", "accountable identity, usually an email address")
	trustCmd.Flags().StringVar(&kind, "kind", "human", "human or machine")
	trustCmd.Flags().StringSliceVar(&roles, "roles", nil, "roles granted to this key, e.g. security,maintainer")
	cmd.AddCommand(trustCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "revoke <key-id>",
		Short: "Revoke a key in the trust store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if !e.Trust.Revoke(args[0]) {
				return fmt.Errorf("no key %s in trust store", args[0])
			}
			if err := e.Trust.Save(e.Project.ConfigPath(project.TrustFile)); err != nil {
				return err
			}
			_, _ = e.Ledger.Append("key.revoked", "human:"+e.Principal, args[0], nil)
			fmt.Fprintf(g.stdout, "revoked %s\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List keys in the trust store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, e.Trust.Keys)
			}
			s := styleFor(g.stdout)
			rows := make([][]string, 0, len(e.Trust.Keys))
			for _, k := range e.Trust.Keys {
				state := s.green("active")
				if k.Revoked {
					state = s.red("revoked")
				}
				rows = append(rows, []string{k.ID, k.Principal, string(k.Kind), strings.Join(k.Roles, ","), state})
			}
			table(g.stdout, []string{"KEY", "PRINCIPAL", "KIND", "ROLES", "STATE"}, rows)
			return nil
		},
	})
	return cmd
}
