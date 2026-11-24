package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ryan-tobin/new-startup/internal/agent"
	"github.com/ryan-tobin/new-startup/internal/attest"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/ledger"
	"github.com/ryan-tobin/new-startup/internal/policy"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/ryan-tobin/new-startup/internal/session"
	"github.com/ryan-tobin/new-startup/internal/workspace"
)

// env is the loaded state of an initialized project.
type env struct {
	Project    *project.Project
	Ledger     *ledger.Ledger
	Workspaces *workspace.Manager
	Sessions   *session.Manager
	Agents     *agent.Registry
	Policy     *policy.Policy
	Trust      *keys.TrustStore
	Key        *keys.PrivateKey
	Principal  string
}

func loadEnv(ctx context.Context, g *globals) (*env, error) {
	p, err := project.Load(ctx, g.dir)
	if err != nil {
		return nil, err
	}
	return buildEnv(ctx, p)
}

func buildEnv(ctx context.Context, p *project.Project) (*env, error) {
	l, err := ledger.Open(p.LedgerPath())
	if err != nil {
		return nil, err
	}
	pol, err := policy.Load(p.ConfigPath(project.PolicyFile))
	if err != nil {
		return nil, err
	}
	trust, err := keys.LoadTrustStore(p.ConfigPath(project.TrustFile))
	if err != nil {
		return nil, err
	}
	key, _, err := keys.LoadOrCreate(project.UserKeyPath())
	if err != nil {
		return nil, fmt.Errorf("load signing key: %w", err)
	}
	principal, _ := principalFor(ctx, p)
	wm := workspace.NewManager(p, l)
	var custom []project.AgentProfile
	if p.Config != nil {
		custom = p.Config.Agents
	}
	return &env{
		Project:    p,
		Ledger:     l,
		Workspaces: wm,
		Sessions:   session.NewManager(wm, l, p.SessionsDir()),
		Agents:     agent.NewRegistry(custom),
		Policy:     pol,
		Trust:      trust,
		Key:        key,
		Principal:  principal,
	}, nil
}

func principalFor(ctx context.Context, p *project.Project) (string, error) {
	if v := os.Getenv("SIGIL_PRINCIPAL"); v != "" {
		return v, nil
	}
	if v := p.Repo.Config(ctx, "user.email"); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("no identity: set git user.email or SIGIL_PRINCIPAL")
}

// useConfigFrom replaces the policy and trust store with the versions committed at rev,
// so that a branch under review cannot relax the rules it is judged by.
func (e *env) useConfigFrom(ctx context.Context, rev string) error {
	if rev == "" {
		return nil
	}
	polData, err := e.Project.Repo.ShowFile(ctx, rev, project.ConfigDirName+"/"+project.PolicyFile)
	if err != nil {
		return fmt.Errorf("read policy at %s: %w", rev, err)
	}
	pol, err := policy.Parse(polData)
	if err != nil {
		return fmt.Errorf("policy at %s: %w", rev, err)
	}
	trustData, err := e.Project.Repo.ShowFile(ctx, rev, project.ConfigDirName+"/"+project.TrustFile)
	if err != nil {
		return fmt.Errorf("read trust store at %s: %w", rev, err)
	}
	trust, err := keys.ParseTrustStore(trustData)
	if err != nil {
		return fmt.Errorf("trust store at %s: %w", rev, err)
	}
	e.Policy, e.Trust = pol, trust
	return nil
}

// requireIdentity fails for commands that sign or own work when no identity is configured.
func (e *env) requireIdentity() error {
	if e.Principal == "" {
		return errors.New("no identity: set git user.email or SIGIL_PRINCIPAL")
	}
	return nil
}

func (e *env) attest() *attest.Service {
	return &attest.Service{
		Workspaces: e.Workspaces,
		Ledger:     e.Ledger,
		Policy:     e.Policy,
		Signer:     e.Key,
		Principal:  e.Principal,
	}
}

// resolveWorkspace picks the named workspace, or the one containing the working directory.
func (e *env) resolveWorkspace(name, dir string) (*workspace.Workspace, error) {
	if name != "" {
		return e.Workspaces.Get(name)
	}
	ws, err := e.Workspaces.ForPath(dir)
	if err != nil {
		return nil, fmt.Errorf("no workspace given and %s is not inside one; pass --ws", dir)
	}
	return ws, nil
}
