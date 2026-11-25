package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/policy"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/spf13/cobra"
)

func newInitCmd(g *globals) *cobra.Command {
	var name string
	var roles []string
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Seralith in the current repository",
		Long: `Initialize Seralith in the current repository.

Creates .seralith/config.yaml, a default policy, and a trust store containing
your signing key. Commit the .seralith directory so that collaborators and CI
verify against the same policy and keys.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p, err := project.Find(ctx, g.dir)
			if err != nil {
				return err
			}
			if p.Config != nil && !force {
				return errors.New("seralith is already initialized here (use --force to rewrite configuration)")
			}
			if name == "" {
				name = filepath.Base(p.Root)
			}
			p.Config = &project.Config{Version: 1, Name: name, Workspaces: project.WorkspaceConfig{Copy: []string{".env", ".env.local"}}}
			if err := p.SaveConfig(); err != nil {
				return err
			}
			if _, err := os.Stat(p.ConfigPath(project.PolicyFile)); errors.Is(err, os.ErrNotExist) || force {
				if err := os.WriteFile(p.ConfigPath(project.PolicyFile), []byte(policy.Default), 0o644); err != nil {
					return err
				}
			}
			key, created, err := keys.LoadOrCreate(project.UserKeyPath())
			if err != nil {
				return err
			}
			principal, err := principalFor(ctx, p)
			if err != nil {
				return err
			}
			trust, err := keys.LoadTrustStore(p.ConfigPath(project.TrustFile))
			if err != nil {
				return err
			}
			if _, err := trust.Add(key.Public(), principal, keys.KindHuman, roles...); err != nil {
				return err
			}
			if err := trust.Save(p.ConfigPath(project.TrustFile)); err != nil {
				return err
			}
			e, err := buildEnv(ctx, p)
			if err != nil {
				return err
			}
			if _, err := e.Ledger.Append("project.initialized", "human:"+principal, name, map[string]any{"key": key.KeyID(), "roles": roles}); err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, map[string]any{"project": name, "root": p.Root, "key": key.KeyID(), "principal": principal, "keyCreated": created})
			}
			s := styleFor(g.stdout)
			fmt.Fprintf(g.stdout, "%s Seralith initialized for %s\n\n", s.green("✓"), s.bold(name))
			fmt.Fprintf(g.stdout, "  config     %s\n", rel(p.Root, p.ConfigPath(project.ConfigFile)))
			fmt.Fprintf(g.stdout, "  policy     %s\n", rel(p.Root, p.ConfigPath(project.PolicyFile)))
			fmt.Fprintf(g.stdout, "  trust      %s\n", rel(p.Root, p.ConfigPath(project.TrustFile)))
			fmt.Fprintf(g.stdout, "  identity   %s %s\n", principal, s.dim(key.KeyID()))
			if created {
				fmt.Fprintf(g.stdout, "  new key    %s\n", project.UserKeyPath())
			}
			fmt.Fprintf(g.stdout, "\nNext:\n  git add .seralith && git commit -m \"chore: enable seralith\"\n  seralith ws new my-task\n  seralith run --ws my-task --agent <tool>\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "project name (defaults to the directory name)")
	cmd.Flags().StringSliceVar(&roles, "roles", []string{"maintainer"}, "roles to grant your key in the trust store")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing configuration and policy")
	return cmd
}

func rel(base, path string) string {
	if r, err := filepath.Rel(base, path); err == nil {
		return r
	}
	return path
}
