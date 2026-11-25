package cli

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os/exec"
	"runtime"

	"github.com/seralith-hq/seralith/internal/daemon"
	"github.com/seralith-hq/seralith/internal/webui"
	"github.com/spf13/cobra"
)

func newServeCmd(g *globals) *cobra.Command {
	var addr, token string
	var open, dev bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the workbench daemon and web UI on localhost",
		Long: `Start the workbench daemon and web UI on localhost.

The daemon binds to the loopback interface only and requires a per-run token,
which is embedded in the printed URL. Requests with a non-loopback Host or a
cross-site Origin are refused.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
				return fmt.Errorf("refusing to listen on non-loopback address %s", addr)
			}
			if token == "" {
				token = daemon.NewToken()
			}
			var ui fs.FS
			if webui.Available() {
				ui = webui.FS()
			}
			cfg := daemon.Config{
				Project: e.Project, Ledger: e.Ledger, Workspaces: e.Workspaces, Sessions: e.Sessions,
				Agents: e.Agents, Policy: e.Policy, Trust: e.Trust, Key: e.Key, Principal: e.Principal,
				UI: ui, Token: token, Logger: slog.New(slog.NewTextHandler(g.stderr, nil)),
			}
			if dev {
				cfg.DevOrigins = []string{"http://localhost:5173", "http://127.0.0.1:5173"}
			}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			url := fmt.Sprintf("http://%s/#token=%s", ln.Addr().String(), token)
			s := styleFor(g.stdout)
			fmt.Fprintf(g.stdout, "%s workbench for %s\n  %s\n", s.accent("◆ seralith"), s.bold(e.Project.Config.Name), url)
			if dev {
				fmt.Fprintf(g.stdout, "  dev UI: http://localhost:5173/#token=%s\n", token)
			}
			if ui == nil {
				fmt.Fprintln(g.stdout, s.dim("  UI not embedded in this build; run `make web build`"))
			}
			if open {
				openBrowser(url)
			}
			_, _ = e.Ledger.Append("daemon.started", "human:"+e.Principal, ln.Addr().String(), nil)
			return daemon.New(cfg).Serve(cmd.Context(), ln)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:7457", "loopback address to listen on")
	cmd.Flags().StringVar(&token, "token", "", "API token (random when empty)")
	cmd.Flags().BoolVar(&open, "open", false, "open the workbench in a browser")
	cmd.Flags().BoolVar(&dev, "dev", false, "allow the Vite dev server origin")
	return cmd
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
