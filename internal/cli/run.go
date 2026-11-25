package cli

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/seralith-hq/seralith/internal/agent"
	"github.com/seralith-hq/seralith/internal/session"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newRunCmd(g *globals) *cobra.Command {
	var wsName, agentID, model string
	var noPTY bool
	cmd := &cobra.Command{
		Use:   "run [--ws name] [--agent id] [-- command args...]",
		Short: "Run a tool or shell inside a workspace as an attributed session",
		Long: `Run a tool or shell inside a workspace as an attributed session.

The workspace is snapshotted before and after the session, and the difference
is recorded as a segment attributed to the session's actor. Without a command,
the agent profile's default command is used. Without --agent, Seralith infers the
profile from the command, and an unrecognized command is attributed to you.`,
		Example: `  seralith run --ws auth --agent codex
  seralith run --ws auth --agent gemini --model pro -- gemini
  seralith run --ws auth -- $SHELL
  seralith run --ws auth --agent aider -- aider --model local/qwen`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			e, err := loadEnv(ctx, g)
			if err != nil {
				return err
			}
			if err := e.requireIdentity(); err != nil {
				return err
			}
			ws, err := e.resolveWorkspace(wsName, g.dir)
			if err != nil {
				return err
			}
			profile, command, err := pickProfile(e.Agents, agentID, args)
			if err != nil {
				return err
			}
			actor := profile.Actor(ctx, e.Principal, command, os.Environ())
			if model != "" {
				actor.Model = model
			}
			stdinFile, interactive := g.stdin.(*os.File)
			interactive = interactive && term.IsTerminal(int(stdinFile.Fd())) && !noPTY

			s := styleFor(g.stderr)
			fmt.Fprintf(g.stderr, "%s %s in %s as %s\n", s.accent("◆ seralith"), joinCommand(command), s.bold(ws.Name), actor.String())

			spec := session.Spec{Workspace: ws.Name, Actor: actor, Command: command, PTY: interactive}
			if !interactive {
				spec.Stdin = g.stdin
			} else if cols, rows, err := term.GetSize(int(stdinFile.Fd())); err == nil {
				spec.Cols, spec.Rows = clampDim(cols), clampDim(rows)
			}
			sess, err := e.Sessions.Start(ctx, spec)
			if err != nil {
				return err
			}
			if interactive {
				attachTerminal(sess, stdinFile, g.stdout)
			} else {
				hist, ch, cancel := sess.Subscribe()
				_, _ = g.stdout.Write(hist)
				for chunk := range ch {
					_, _ = g.stdout.Write(chunk)
				}
				cancel()
			}
			<-sess.Done()
			info := sess.Info()
			if info.Error != "" {
				return fmt.Errorf("session %s: %s", info.ID, info.Error)
			}
			fmt.Fprintf(g.stderr, "%s session %s exited %d · %s changed (+%d −%d)\n",
				s.accent("◆ seralith"), short(info.ID), *info.ExitCode, plural(info.FilesChanged, "file"), info.LinesAdded, info.LinesDeleted)
			if *info.ExitCode != 0 {
				return &ExitError{Code: *info.ExitCode}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&wsName, "ws", "", "workspace name (defaults to the workspace containing the current directory)")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent profile id; see `seralith agents`")
	cmd.Flags().StringVar(&model, "model", "", "declare the model in use, for the record")
	cmd.Flags().BoolVar(&noPTY, "no-pty", false, "use pipes instead of a pseudo-terminal")
	return cmd
}

func pickProfile(reg *agent.Registry, id string, args []string) (agent.Profile, []string, error) {
	if id != "" {
		p, ok := reg.Get(id)
		if !ok {
			return agent.Profile{}, nil, fmt.Errorf("unknown agent %q; see `seralith agents`", id)
		}
		if len(args) == 0 {
			args = p.Command
		}
		return p, args, nil
	}
	if len(args) == 0 {
		p, _ := reg.Get("shell")
		return p, p.Command, nil
	}
	if p, ok := reg.Guess(args); ok {
		return p, args, nil
	}
	p, _ := reg.Get("shell")
	return p, args, nil
}

func attachTerminal(sess *session.Session, stdin *os.File, stdout io.Writer) {
	fd := int(stdin.Fd())
	if state, err := term.MakeRaw(fd); err == nil {
		defer func() { _ = term.Restore(fd, state) }()
	}
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for range winch {
			if cols, rows, err := term.GetSize(fd); err == nil {
				_ = sess.Resize(clampDim(cols), clampDim(rows))
			}
		}
	}()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				if _, werr := sess.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	hist, ch, cancel := sess.Subscribe()
	defer cancel()
	_, _ = stdout.Write(hist)
	for chunk := range ch {
		_, _ = stdout.Write(chunk)
	}
}

func clampDim(v int) uint16 {
	return uint16(min(max(v, 1), 1<<15)) //nolint:gosec // bounded to [1, 32768]
}
