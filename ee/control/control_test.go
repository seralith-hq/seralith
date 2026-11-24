package control_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryan-tobin/new-startup/ee/control"
	"github.com/ryan-tobin/new-startup/internal/cli"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/policy"
	"github.com/ryan-tobin/new-startup/internal/testutil"
	"github.com/ryan-tobin/new-startup/pkg/dsse"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

const bootstrap = "bootstrap-secret"

type api struct {
	t   *testing.T
	url string
}

func newAPI(t *testing.T) (*api, *control.Store) {
	t.Helper()
	store, err := control.Open(context.Background(), filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := httptest.NewServer(control.NewServer(control.ServerConfig{Store: store, BootstrapToken: bootstrap, Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return &api{t: t, url: srv.URL}, store
}

func (a *api) call(method, path, token string, body any, out any, headers ...string) int {
	a.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, a.url+path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil {
		if raw, ok := out.(*string); ok {
			*raw = string(data)
		} else {
			_ = json.Unmarshal(data, out)
		}
	}
	return resp.StatusCode
}

func (a *api) bootstrapOrg() string {
	a.t.Helper()
	var res struct {
		Token string `json:"token"`
	}
	code := a.call("POST", "/v1/bootstrap", "", map[string]string{"org": "Acme Defense", "slug": "acme", "email": "owner@acme.dev"}, &res, "X-Bootstrap-Token", bootstrap)
	if code != http.StatusCreated || res.Token == "" {
		a.t.Fatalf("bootstrap: %d", code)
	}
	return res.Token
}

func TestBootstrapIsGuardedAndOneShot(t *testing.T) {
	a, _ := newAPI(t)
	if code := a.call("POST", "/v1/bootstrap", "", map[string]string{"org": "x", "slug": "xx", "email": "a@b.c"}, nil, "X-Bootstrap-Token", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("wrong bootstrap token: %d", code)
	}
	a.bootstrapOrg()
	if code := a.call("POST", "/v1/bootstrap", "", map[string]string{"org": "y", "slug": "yy", "email": "a@b.c"}, nil, "X-Bootstrap-Token", bootstrap); code != http.StatusConflict {
		t.Fatalf("second bootstrap: %d", code)
	}
	if code := a.call("GET", "/v1/me", "sgl_bogus", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("bogus token: %d", code)
	}
}

func TestRoleEnforcementAndAudit(t *testing.T) {
	a, _ := newAPI(t)
	owner := a.bootstrapOrg()
	if code := a.call("POST", "/v1/projects", owner, map[string]string{"name": "radar-fw"}, nil); code != http.StatusCreated {
		t.Fatalf("create project: %d", code)
	}
	var added struct {
		Token string `json:"token"`
	}
	if code := a.call("POST", "/v1/members", owner, map[string]string{"email": "ci@acme.dev", "role": "ci"}, &added); code != http.StatusCreated || added.Token == "" {
		t.Fatalf("add ci member: %d", code)
	}
	if code := a.call("POST", "/v1/members", owner, map[string]string{"email": "audit@acme.dev", "role": "auditor"}, nil); code != http.StatusCreated {
		t.Fatalf("add auditor: %d", code)
	}
	if code := a.call("POST", "/v1/members", owner, map[string]string{"email": "x@acme.dev", "role": "owner"}, nil); code != http.StatusBadRequest {
		t.Fatalf("granting owner should be refused: %d", code)
	}
	if code := a.call("POST", "/v1/projects", added.Token, map[string]string{"name": "other"}, nil); code != http.StatusForbidden {
		t.Fatalf("ci creating project: %d", code)
	}
	if code := a.call("GET", "/v1/audit", added.Token, nil, nil); code != http.StatusForbidden {
		t.Fatalf("ci reading audit: %d", code)
	}
	if code := a.call("PUT", "/v1/projects/radar-fw/policy", owner, map[string]string{"document": "version: 2\n"}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid policy: %d", code)
	}
	var events []control.AuditEvent
	a.call("GET", "/v1/audit", owner, nil, &events)
	if len(events) != 4 || events[0].Action != "org.created" {
		t.Fatalf("audit = %+v", events)
	}
	var rep control.AuditReport
	a.call("GET", "/v1/audit/verify", owner, nil, &rep)
	if rep.Events != 4 || len(rep.Issues) != 0 {
		t.Fatalf("audit verify = %+v", rep)
	}
	var metrics string
	a.call("GET", "/metrics", "", nil, &metrics)
	if !strings.Contains(metrics, "sigil_control_requests_total") {
		t.Fatalf("metrics = %s", metrics)
	}
}

func TestIngestVerifiesAndSummarizes(t *testing.T) {
	a, _ := newAPI(t)
	owner := a.bootstrapOrg()
	a.call("POST", "/v1/projects", owner, map[string]string{"name": "radar-fw"}, nil)
	a.call("PUT", "/v1/projects/radar-fw/policy", owner, map[string]string{"document": policy.Default}, nil)

	dev, _ := keys.Generate()
	reviewer, _ := keys.Generate()
	stranger, _ := keys.Generate()
	for _, k := range []struct {
		key       *keys.PrivateKey
		principal string
		roles     []string
	}{{dev, "dev@acme.dev", nil}, {reviewer, "sec@acme.dev", []string{"security"}}} {
		pemText, _ := k.key.Public().PEM()
		if code := a.call("POST", "/v1/keys", owner, map[string]any{"principal": k.principal, "kind": "human", "roles": k.roles, "publicKey": pemText}, nil); code != http.StatusCreated {
			t.Fatalf("add key: %d", code)
		}
	}

	change := func(commit string, signer *keys.PrivateKey, path string) *dsse.Envelope {
		pred := provenance.ChangePredicate{
			Commit:    commit,
			Committer: provenance.Actor{Kind: provenance.ActorHuman, ID: "dev@acme.dev"},
			Segments: []provenance.Segment{{
				ID: "s1", Kind: provenance.SegmentSession,
				Actor: provenance.Actor{Kind: provenance.ActorAgent, ID: "codex", Tool: "codex", Model: "m-1"},
				Files: []provenance.FileChange{{Path: path, Status: provenance.FileAdded, LinesAdded: 30}},
			}},
			Summary: provenance.Summary{LinesByKind: map[provenance.ActorKind]int{provenance.ActorAgent: 30, provenance.ActorHuman: 10}},
		}
		st, _ := provenance.NewStatement(commit, provenance.PredicateChange, pred)
		env, _ := st.Seal(signer)
		return env
	}
	c1 := strings.Repeat("a", 40)
	c2 := strings.Repeat("b", 40)
	envs := []*dsse.Envelope{change(c1, dev, "internal/crypto/aes.go"), change(c2, dev, "docs/readme.md"), change(strings.Repeat("c", 40), stranger, "x.go")}
	var res struct {
		Stored    int `json:"stored"`
		Untrusted int `json:"untrusted"`
	}
	if code := a.call("POST", "/v1/projects/radar-fw/attestations", owner, map[string]any{"envelopes": envs}, &res); code != http.StatusOK || res.Stored != 3 || res.Untrusted != 1 {
		t.Fatalf("ingest: %d %+v", code, res)
	}
	a.call("POST", "/v1/projects/radar-fw/attestations", owner, map[string]any{"envelopes": envs}, &res)
	if res.Stored != 0 {
		t.Fatal("re-ingesting stored duplicates")
	}

	var stats control.Stats
	a.call("GET", "/v1/projects/radar-fw/stats", owner, nil, &stats)
	if stats.Commits != 3 || stats.Untrusted != 1 || stats.AgentLines != 60 || stats.ByTool["codex"] != 2 || len(stats.AwaitingApproval) != 1 || stats.AwaitingApproval[0] != c1 {
		t.Fatalf("stats = %+v", stats)
	}

	ap, _ := provenance.NewStatement(c1, provenance.PredicateApproval, provenance.ApprovalPredicate{Commit: c1, Approver: provenance.Actor{Kind: provenance.ActorHuman, ID: "sec@acme.dev"}, Role: "security", Decision: provenance.Approve})
	apEnv, _ := ap.Seal(reviewer)
	a.call("POST", "/v1/projects/radar-fw/attestations", owner, map[string]any{"envelopes": []*dsse.Envelope{apEnv}}, nil)
	a.call("GET", "/v1/projects/radar-fw/stats", owner, nil, &stats)
	if len(stats.AwaitingApproval) != 0 {
		t.Fatalf("approval not recognized: %+v", stats)
	}

	var list []control.AttestationRecord
	a.call("GET", "/v1/projects/radar-fw/attestations?commit="+c1+"&envelope=true", owner, nil, &list)
	if len(list) != 2 || list[0].Envelope == "" {
		t.Fatalf("attestations for commit = %+v", list)
	}

	var trustYAML string
	a.call("GET", "/v1/trust.yaml", owner, nil, &trustYAML)
	ts, err := keys.ParseTrustStore([]byte(trustYAML))
	if err != nil || len(ts.Keys) != 2 {
		t.Fatalf("trust export: %v %d", err, len(ts.Keys))
	}
	if code := a.call("POST", "/v1/keys/"+dev.KeyID()+"/revoke", owner, nil, nil); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
}

func TestCLICloudRoundTrip(t *testing.T) {
	a, _ := newAPI(t)
	owner := a.bootstrapOrg()
	a.call("POST", "/v1/projects", owner, map[string]string{"name": "demo"}, nil)
	a.call("PUT", "/v1/projects/demo/policy", owner, map[string]string{"document": policy.Default}, nil)

	home := t.TempDir()
	t.Setenv("SIGIL_DATA_DIR", filepath.Join(home, "data"))
	t.Setenv("SIGIL_CONFIG_DIR", filepath.Join(home, "config"))
	t.Setenv("SIGIL_KEY", filepath.Join(home, "dev.pem"))
	t.Setenv("SIGIL_PRINCIPAL", "dev@acme.dev")
	g := testutil.NewGitRepo(t)
	g.Write("a.txt", "a\n")
	g.Commit("init")

	run := func(args ...string) string {
		t.Helper()
		var out, errb bytes.Buffer
		if code := cli.Main(append([]string{"-C", g.Dir}, args...), strings.NewReader(""), &out, &errb); code != 0 {
			t.Fatalf("sigil %v: %d\n%s%s", args, code, out.String(), errb.String())
		}
		return out.String()
	}
	run("init")
	pub := run("key", "export")
	a.call("POST", "/v1/keys", owner, map[string]any{"principal": "dev@acme.dev", "kind": "human", "roles": []string{"maintainer"}, "publicKey": pub}, nil)
	g.Commit("chore: enable sigil")
	run("ws", "new", "w")
	run("run", "--ws", "w", "--agent", "codex", "--", "sh", "-c", "echo hi > b.txt")
	run("commit", "--ws", "w", "-m", "feat: b")

	if out := run("cloud", "login", "--url", a.url, "--token", owner, "--project", "demo"); !strings.Contains(out, "Acme Defense") {
		t.Fatalf("login: %s", out)
	}
	if out := run("cloud", "push"); !strings.Contains(out, "pushed 1 attestation") || strings.Contains(out, "untrusted") {
		t.Fatalf("push: %s", out)
	}
	if out := run("cloud", "pull-trust"); !strings.Contains(out, "1 key") {
		t.Fatalf("pull-trust: %s", out)
	}
	if out := run("cloud", "pull-policy"); !strings.Contains(out, "5 rules") {
		t.Fatalf("pull-policy: %s", out)
	}
	if out := run("cloud", "stats"); !strings.Contains(out, `"commits": 1`) {
		t.Fatalf("stats: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "config", "cloud.json")); err != nil {
		t.Fatal("credentials not saved")
	}
}
