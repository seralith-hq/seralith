package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ryan-tobin/new-startup/internal/agent"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/ledger"
	"github.com/ryan-tobin/new-startup/internal/policy"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/ryan-tobin/new-startup/internal/session"
	"github.com/ryan-tobin/new-startup/internal/testutil"
	"github.com/ryan-tobin/new-startup/internal/workspace"
)

const token = "test-token"

type fixture struct {
	t   *testing.T
	srv *httptest.Server
	s   *Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("SERALITH_DATA_DIR", t.TempDir())
	g := testutil.NewGitRepo(t)
	g.Write(".seralith/config.yaml", "version: 1\nname: demo\n")
	g.Write(".seralith/policy.yaml", policy.Default)
	g.Write("README.md", "# demo\n")
	g.Commit("init")
	ctx := context.Background()
	p, err := project.Load(ctx, g.Dir)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := ledger.Open(p.LedgerPath())
	pol, _ := policy.Load(p.ConfigPath(project.PolicyFile))
	k, _ := keys.Generate()
	ts := keys.NewTrustStore()
	if _, err := ts.Add(k.Public(), "dev@example.com", keys.KindHuman, "maintainer"); err != nil {
		t.Fatal(err)
	}
	wm := workspace.NewManager(p, l)
	s := New(Config{
		Project: p, Ledger: l, Workspaces: wm, Sessions: session.NewManager(wm, l, p.SessionsDir()),
		Agents: agent.NewRegistry(nil), Policy: pol, Trust: ts, Key: k, Principal: "dev@example.com", Token: token,
	})
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return &fixture{t: t, srv: srv, s: s}
}

func (f *fixture) do(method, path string, body any, out any) int {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, f.srv.URL+path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestAuthGuards(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Get(f.srv.URL + "/api/v1/project")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: %d", resp.StatusCode)
	}

	req, _ := http.NewRequest("GET", f.srv.URL+"/api/v1/project", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://evil.example")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross origin: %d", resp.StatusCode)
	}

	req, _ = http.NewRequest("GET", f.srv.URL+"/api/v1/project", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Host = "attacker.example:7457"
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("rebinding host: %d", resp.StatusCode)
	}

	var project map[string]any
	if code := f.do("GET", "/api/v1/project", nil, &project); code != 200 || project["name"] != "demo" || project["trusted"] != true {
		t.Fatalf("project: %d %v", code, project)
	}
	resp, err = http.Get(f.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP header")
	}
}

func TestWorkspaceSessionCommitFlow(t *testing.T) {
	f := newFixture(t)
	if code := f.do("POST", "/api/v1/workspaces", map[string]string{"name": "api"}, nil); code != http.StatusCreated {
		t.Fatalf("create workspace: %d", code)
	}
	if code := f.do("POST", "/api/v1/workspaces", map[string]string{"name": "api"}, nil); code != http.StatusConflict {
		t.Fatalf("duplicate workspace: %d", code)
	}

	events, cancel := f.s.events.subscribe()
	defer cancel()

	var info session.Info
	code := f.do("POST", "/api/v1/sessions", map[string]any{
		"workspace": "api", "agent": "codex", "model": "m1",
		"command": []string{"sh", "-c", "sleep 0.3; printf 'hello\\n' > hello.txt; echo finished"},
	}, &info)
	if code != http.StatusCreated || info.Actor.Model != "m1" || info.Actor.Kind != "agent" {
		t.Fatalf("start session: %d %+v", code, info)
	}

	wsURL := "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/api/v1/sessions/" + info.ID + "/pty?token=" + token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	defer conn.Close()
	_ = conn.WriteJSON(ptyMessage{Type: "resize", Cols: 100, Rows: 30})
	var output strings.Builder
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		kind, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if kind == websocket.BinaryMessage {
			output.Write(data)
		} else if strings.Contains(string(data), `"exit"`) {
			break
		}
	}
	if !strings.Contains(output.String(), "finished") {
		t.Fatalf("pty output = %q", output.String())
	}

	waitFor(t, func() bool {
		var got session.Info
		f.do("GET", "/api/v1/sessions/"+info.ID, nil, &got)
		return got.State == session.StateExited
	})
	sawSession := false
	for len(events) > 0 {
		if e := <-events; e.Type == "session" {
			sawSession = true
		}
	}
	if !sawSession {
		t.Fatal("no session events published")
	}

	var diff struct {
		Files []struct{ Path string } `json:"files"`
		Patch string                  `json:"patch"`
	}
	f.do("GET", "/api/v1/workspaces/api/diff", nil, &diff)
	if len(diff.Files) != 1 || diff.Files[0].Path != "hello.txt" || !strings.Contains(diff.Patch, "+hello") {
		t.Fatalf("diff = %+v", diff)
	}

	var status map[string]any
	if code := f.do("GET", "/api/v1/workspaces/api/status", nil, &status); code != 200 || len(status["Segments"].([]any)) != 1 {
		t.Fatalf("status: %d %v", code, status)
	}

	if code := f.do("POST", "/api/v1/workspaces/api/commit", map[string]string{"message": ""}, nil); code != http.StatusBadRequest {
		t.Fatalf("empty message: %d", code)
	}
	var commit map[string]any
	if code := f.do("POST", "/api/v1/workspaces/api/commit", map[string]string{"message": "feat: hello"}, &commit); code != http.StatusCreated {
		t.Fatalf("commit: %d %v", code, commit)
	}
	sha := commit["commit"].(string)

	var rows []commitRow
	f.do("GET", "/api/v1/commits", nil, &rows)
	if len(rows) != 1 || rows[0].Commit != sha || rows[0].Status != "pass" {
		t.Fatalf("commits = %+v", rows)
	}
	var verified struct {
		Report struct {
			Status string `json:"status"`
		} `json:"report"`
	}
	f.do("GET", "/api/v1/commits/"+sha, nil, &verified)
	if verified.Report.Status != "pass" {
		t.Fatalf("verify = %+v", verified)
	}
	if code := f.do("POST", "/api/v1/commits/"+sha+"/approve", map[string]any{"comment": "lgtm"}, nil); code != http.StatusCreated {
		t.Fatalf("approve: %d", code)
	}

	var entries []ledger.Entry
	f.do("GET", "/api/v1/ledger?type=approval.recorded", nil, &entries)
	if len(entries) != 1 {
		t.Fatalf("ledger = %+v", entries)
	}
	var rep ledger.Report
	f.do("GET", "/api/v1/ledger/verify", nil, &rep)
	if !rep.OK() {
		t.Fatalf("ledger verify = %+v", rep)
	}
	var blame struct {
		Summary struct {
			Lines int `json:"lines"`
		} `json:"summary"`
	}
	f.do("GET", "/api/v1/blame?rev="+sha+"&path=hello.txt", nil, &blame)
	if blame.Summary.Lines != 1 {
		t.Fatalf("blame = %+v", blame)
	}
	if code := f.do("DELETE", "/api/v1/workspaces/api", nil, nil); code != http.StatusNoContent {
		t.Fatalf("remove: %d", code)
	}
}

func TestStartSessionValidation(t *testing.T) {
	f := newFixture(t)
	if code := f.do("POST", "/api/v1/sessions", map[string]any{"workspace": "missing"}, nil); code != http.StatusNotFound {
		t.Fatalf("missing workspace: %d", code)
	}
	if code := f.do("POST", "/api/v1/sessions", map[string]any{"workspace": "x", "agent": "nope"}, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown agent: %d", code)
	}
	if code := f.do("POST", "/api/v1/sessions", map[string]any{"bogus": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", code)
	}
	if code := f.do("GET", "/api/v1/gate", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("gate without range: %d", code)
	}
}

func TestLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1:7457": true, "localhost:7457": true, "[::1]:7457": true, "localhost": true,
		"evil.example": false, "192.168.1.5:7457": false, "127.0.0.1.evil.example": false,
	} {
		if got := loopbackHost(host); got != want {
			t.Errorf("loopbackHost(%q) = %v", host, got)
		}
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}
