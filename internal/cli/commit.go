package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ryan-tobin/new-startup/internal/attest"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
	"github.com/spf13/cobra"
)

func newStatusCmd(g *globals) *cobra.Command {
	var wsName string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show pending segments, attribution, and policy for a workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			ws, err := e.resolveWorkspace(wsName, g.dir)
			if err != nil {
				return err
			}
			pv, err := e.attest().Preview(cmd.Context(), ws.Name)
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, pv)
			}
			printPreview(g.stdout, pv)
			return nil
		},
	}
	cmd.Flags().StringVar(&wsName, "ws", "", "workspace name")
	return cmd
}

func printPreview(w io.Writer, pv *attest.Preview) {
	s := styleFor(w)
	fmt.Fprintf(w, "%s %s  %s\n\n", s.bold("workspace"), pv.Workspace.Name, s.dim(pv.Workspace.Branch))
	if len(pv.Segments) == 0 {
		fmt.Fprintln(w, "nothing pending")
		return
	}
	rows := make([][]string, 0, len(pv.Segments))
	for _, seg := range pv.Segments {
		add, del := 0, 0
		for _, f := range seg.Files {
			add += f.LinesAdded
			del += f.LinesDeleted
		}
		rows = append(rows, []string{short(seg.ID), string(seg.Kind), actorLabel(seg.Actor), plural(len(seg.Files), "file"), fmt.Sprintf("+%d −%d", add, del)})
	}
	table(w, []string{"SEGMENT", "KIND", "ACTOR", "FILES", "LINES"}, rows)
	fmt.Fprintf(w, "\n%s %s\n", s.bold("attribution"), formatLines(pv.Summary.LinesByKind))
	printDecisions(w, pv.Policy.Decisions)
}

func printDecisions(w io.Writer, decisions []provenance.Decision) {
	s := styleFor(w)
	if len(decisions) == 0 {
		fmt.Fprintf(w, "%s %s\n", s.bold("policy"), s.green("no rules triggered"))
		return
	}
	fmt.Fprintln(w, s.bold("policy"))
	for _, d := range decisions {
		extra := ""
		if d.Effect == provenance.EffectRequireApproval {
			extra = fmt.Sprintf(" (%s", plural(d.Approvals, "approval"))
			if len(d.Roles) > 0 {
				extra += " from " + strings.Join(d.Roles, "/")
			}
			extra += ")"
		}
		fmt.Fprintf(w, "  %-18s %s%s\n    %s\n    %s\n", s.status(string(d.Effect)), d.Rule, extra, d.Message, s.dim(strings.Join(d.Paths, ", ")))
	}
}

func actorLabel(a provenance.Actor) string {
	label := string(a.Kind) + ":" + a.ID
	if a.Model != "" {
		label += " [" + a.Model + "]"
	}
	return label
}

func formatLines(m map[provenance.ActorKind]int) string {
	if len(m) == 0 {
		return "no surviving lines"
	}
	keys := make([]string, 0, len(m))
	total := 0
	for k, v := range m {
		keys = append(keys, string(k))
		total += v
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := m[provenance.ActorKind(k)]
		parts = append(parts, fmt.Sprintf("%s %d (%d%%)", k, v, v*100/max(total, 1)))
	}
	return strings.Join(parts, " · ")
}

func newCommitCmd(g *globals) *cobra.Command {
	var wsName, message string
	var force bool
	cmd := &cobra.Command{
		Use:   "commit -m <message>",
		Short: "Commit a workspace and attach a signed change attestation",
		Long: `Commit a workspace and attach a signed change attestation.

All changes in the workspace are staged and committed. The attestation records
every segment since the previous commit, line-level attribution, and the policy
decisions in effect, and is stored in refs/notes/sigil. Push it with
'sigil sync push'.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if message == "" {
				return errors.New("a commit message is required (-m)")
			}
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			ws, err := e.resolveWorkspace(wsName, g.dir)
			if err != nil {
				return err
			}
			res, err := e.attest().Commit(cmd.Context(), attest.CommitOptions{Workspace: ws.Name, Message: message, Force: force})
			if errors.Is(err, attest.ErrDenied) {
				pv, perr := e.attest().Preview(cmd.Context(), ws.Name)
				if perr == nil {
					printDecisions(g.stderr, pv.Policy.Decisions)
				}
				return &ExitError{Code: 3, Err: fmt.Errorf("%w; fix the change or pass --force to override (recorded in the ledger)", err)}
			}
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(g.stdout, map[string]any{"commit": res.Commit, "attestation": res.Digest, "summary": res.Predicate.Summary, "policy": res.Predicate.Policy})
			}
			s := styleFor(g.stdout)
			fmt.Fprintf(g.stdout, "%s %s %s\n", s.green("✓"), s.bold(short(res.Commit)), message)
			fmt.Fprintf(g.stdout, "  attestation  %s\n", s.dim(res.Digest))
			fmt.Fprintf(g.stdout, "  segments     %d\n", res.Predicate.Summary.Segments)
			fmt.Fprintf(g.stdout, "  attribution  %s\n", formatLines(res.Predicate.Summary.LinesByKind))
			printDecisions(g.stdout, res.Predicate.Policy.Decisions)
			return nil
		},
	}
	cmd.Flags().StringVar(&wsName, "ws", "", "workspace name")
	cmd.Flags().StringVarP(&message, "message", "m", "", "commit message")
	cmd.Flags().BoolVar(&force, "force", false, "commit despite a deny decision; the override is recorded")
	return cmd
}

func newApproveCmd(g *globals) *cobra.Command {
	var role, comment string
	var rules []string
	var reject bool
	cmd := &cobra.Command{
		Use:   "approve <commit>",
		Short: "Record a signed approval (or rejection) of a commit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd.Context(), g)
			if err != nil {
				return err
			}
			env, commit, err := e.attest().Approve(cmd.Context(), e.Project.Repo, attest.ApproveOptions{Rev: args[0], Role: role, Rules: rules, Reject: reject, Comment: comment})
			if err != nil {
				return err
			}
			verb := "approved"
			if reject {
				verb = "rejected"
			}
			if g.json {
				return writeJSON(g.stdout, map[string]any{"commit": commit, "decision": verb, "attestation": provenance.Digest(env)})
			}
			s := styleFor(g.stdout)
			fmt.Fprintf(g.stdout, "%s %s %s as %s", s.green("✓"), verb, short(commit), e.Principal)
			if role != "" {
				fmt.Fprintf(g.stdout, " (role %s)", role)
			}
			fmt.Fprintln(g.stdout)
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "role you are approving in, e.g. security")
	cmd.Flags().StringSliceVar(&rules, "rule", nil, "limit the approval to specific policy rules")
	cmd.Flags().BoolVar(&reject, "reject", false, "record a rejection instead of an approval")
	cmd.Flags().StringVarP(&comment, "message", "m", "", "review comment")
	return cmd
}
