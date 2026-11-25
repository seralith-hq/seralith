package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/seralith-hq/seralith/internal/cloud"
	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/internal/notes"
	"github.com/seralith-hq/seralith/internal/policy"
	"github.com/seralith-hq/seralith/internal/project"
	"github.com/seralith-hq/seralith/pkg/dsse"
	"github.com/spf13/cobra"
)

func credentialsPath() string { return filepath.Join(project.ConfigHome(), "cloud.json") }

func newCloudCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{Use: "cloud", Short: "Connect to Seralith Control (Team, Enterprise, and Sovereign)"}

	var url, token, projectName string
	login := &cobra.Command{
		Use:   "login --url <url> --token <token> [--project <name>]",
		Short: "Save Seralith Control credentials and link this repository to a project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if url == "" || token == "" {
				return errors.New("--url and --token are required")
			}
			creds := &cloud.Credentials{URL: url, Token: token}
			client, err := cloud.New(creds)
			if err != nil {
				return err
			}
			me, err := client.Me(cmd.Context())
			if err != nil {
				return err
			}
			if err := cloud.SaveCredentials(credentialsPath(), creds); err != nil {
				return err
			}
			if projectName != "" {
				p, err := project.Load(cmd.Context(), g.dir)
				if err != nil {
					return err
				}
				p.Config.Control = &project.ControlConfig{URL: url, Project: projectName}
				if err := p.SaveConfig(); err != nil {
					return err
				}
			}
			fmt.Fprintf(g.stdout, "%s logged in to %s as %s (%s)\n", styleFor(g.stdout).green("✓"), me.Org.Name, me.User.Email, me.User.Role)
			return nil
		},
	}
	login.Flags().StringVar(&url, "url", "", "Seralith Control URL")
	login.Flags().StringVar(&token, "token", "", "API token")
	login.Flags().StringVar(&projectName, "project", "", "link this repository to a project")

	var rangeSpec string
	push := &cobra.Command{
		Use:   "push [--range a..b]",
		Short: "Upload attestations to Seralith Control",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, client, name, err := cloudEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			commits, err := attestedCommits(cmd.Context(), e, rangeSpec)
			if err != nil {
				return err
			}
			var envs []*dsse.Envelope
			for _, c := range commits {
				list, err := notes.Read(cmd.Context(), e.Project.Repo, c)
				if err != nil {
					return err
				}
				envs = append(envs, list...)
			}
			if len(envs) == 0 {
				fmt.Fprintln(g.stdout, "no attestations to push")
				return nil
			}
			res, err := client.Push(cmd.Context(), name, envs)
			if err != nil {
				return err
			}
			_, _ = e.Ledger.Append("cloud.pushed", "human:"+e.Principal, name, map[string]int{"sent": len(envs), "stored": res.Stored})
			fmt.Fprintf(g.stdout, "pushed %s from %s (%d new", plural(len(envs), "attestation"), plural(len(commits), "commit"), res.Stored)
			if res.Untrusted > 0 {
				fmt.Fprintf(g.stdout, ", %s", styleFor(g.stdout).red(fmt.Sprintf("%d untrusted", res.Untrusted)))
			}
			fmt.Fprintln(g.stdout, ")")
			return nil
		},
	}
	push.Flags().StringVar(&rangeSpec, "range", "", "only commits in this range (default: every attested commit)")

	pullTrust := &cobra.Command{
		Use:   "pull-trust",
		Short: "Replace .seralith/trust.yaml with the organization key registry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, client, _, err := cloudEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			data, err := client.TrustYAML(cmd.Context())
			if err != nil {
				return err
			}
			ts, err := keys.ParseTrustStore(data)
			if err != nil {
				return err
			}
			if err := os.WriteFile(e.Project.ConfigPath(project.TrustFile), data, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(g.stdout, "wrote %s (%s)\n", project.TrustFile, plural(len(ts.Keys), "key"))
			return nil
		},
	}

	pullPolicy := &cobra.Command{
		Use:   "pull-policy",
		Short: "Replace .seralith/policy.yaml with the project's published policy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, client, name, err := cloudEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			data, err := client.PolicyYAML(cmd.Context(), name)
			if err != nil {
				return err
			}
			p, err := policy.Parse(data)
			if err != nil {
				return err
			}
			if err := os.WriteFile(e.Project.ConfigPath(project.PolicyFile), data, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(g.stdout, "wrote %s (%s, sha256 %s)\n", project.PolicyFile, plural(len(p.Rules), "rule"), short(p.Digest()))
			return nil
		},
	}

	stats := &cobra.Command{
		Use:   "stats",
		Short: "Show organization-level attribution and approval statistics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, client, name, err := cloudEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			st, err := client.Stats(cmd.Context(), name)
			if err != nil {
				return err
			}
			return writeJSON(g.stdout, st)
		},
	}

	cmd.AddCommand(login, push, pullTrust, pullPolicy, stats)
	return cmd
}

func cloudEnv(ctx context.Context, g *globals) (*env, *cloud.Client, string, error) {
	e, err := loadEnv(ctx, g)
	if err != nil {
		return nil, nil, "", err
	}
	creds, err := cloud.LoadCredentials(credentialsPath())
	if err != nil {
		return nil, nil, "", err
	}
	client, err := cloud.New(creds)
	if err != nil {
		return nil, nil, "", err
	}
	name := os.Getenv("SERALITH_CLOUD_PROJECT")
	if name == "" && e.Project.Config.Control != nil {
		name = e.Project.Config.Control.Project
	}
	if name == "" {
		return nil, nil, "", errors.New("no project linked; run `seralith cloud login --project <name>` or set SERALITH_CLOUD_PROJECT")
	}
	return e, client, name, nil
}

func attestedCommits(ctx context.Context, e *env, rangeSpec string) ([]string, error) {
	if rangeSpec != "" {
		return e.Project.Repo.RevList(ctx, rangeSpec)
	}
	out, err := e.Project.Repo.Run(ctx, "notes", "--ref="+notes.Ref, "list")
	if err != nil {
		return nil, nil
	}
	var commits []string
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			commits = append(commits, f[1])
		}
	}
	sort.Strings(commits)
	return commits, nil
}
