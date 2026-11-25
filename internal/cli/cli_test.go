package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/internal/testutil"
)

type harness struct {
	t    *testing.T
	repo *testutil.GitRepo
	keys string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	home := t.TempDir()
	t.Setenv("SERALITH_DATA_DIR", filepath.Join(home, "data"))
	t.Setenv("SERALITH_CONFIG_DIR", filepath.Join(home, "config"))
	t.Setenv("NO_COLOR", "1")
	h := &harness{t: t, repo: testutil.NewGitRepo(t), keys: filepath.Join(home, "keys")}
	h.as("alice")
	h.repo.Write("README.md", "# demo\n")
	h.repo.Write("src/app.go", testutil.Lines("package src", "", "func App() {}"))
	h.repo.Commit("initial")
	return h
}

func (h *harness) as(user string) {
	h.t.Setenv("SERALITH_KEY", filepath.Join(h.keys, user+".pem"))
	h.t.Setenv("SERALITH_PRINCIPAL", user+"@example.com")
}

func (h *harness) run(dir string, args ...string) (string, string, int) {
	h.t.Helper()
	var out, errb bytes.Buffer
	if dir == "" {
		dir = h.repo.Dir
	}
	code := Main(append([]string{"-C", dir}, args...), strings.NewReader(""), &out, &errb)
	return out.String(), errb.String(), code
}

func (h *harness) ok(dir string, args ...string) string {
	h.t.Helper()
	out, errOut, code := h.run(dir, args...)
	if code != 0 {
		h.t.Fatalf("seralith %s exited %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

func (h *harness) wsPath(name string) string {
	return strings.TrimSpace(h.ok("", "ws", "path", name))
}

func TestEndToEndWorkflow(t *testing.T) {
	h := newHarness(t)
	out := h.ok("", "init", "--name", "demo")
	if !strings.Contains(out, "Seralith initialized for demo") {
		t.Fatalf("init output: %s", out)
	}
	if _, _, code := h.run("", "init"); code == 0 {
		t.Fatal("second init should fail without --force")
	}
	base := h.repo.Commit("chore: enable seralith")

	h.ok("", "ws", "new", "auth")
	ws := h.wsPath("auth")
	h.ok("", "run", "--ws", "auth", "--agent", "codex", "--model", "test-model", "--", "sh", "-c",
		"mkdir -p internal/auth && printf 'package auth\\n\\nfunc Token() string { return \"t\" }\\n' > internal/auth/token.go && echo done")
	testutil.WriteFile(t, filepath.Join(ws, "src/app.go"), testutil.Lines("package src", "", "// reviewed by a human", "func App() {}"))

	var status struct {
		Segments []struct {
			Kind  string `json:"kind"`
			Actor struct {
				Kind  string `json:"kind"`
				Model string `json:"model"`
			} `json:"actor"`
		}
		Effect string `json:"Effect"`
	}
	if err := json.Unmarshal([]byte(h.ok(ws, "--json", "status")), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Segments) != 2 || status.Segments[0].Actor.Kind != "agent" || status.Segments[0].Actor.Model != "test-model" || status.Segments[1].Kind != "manual" {
		t.Fatalf("status = %+v", status)
	}
	if status.Effect != "require-approval" {
		t.Fatalf("effect = %s", status.Effect)
	}

	out = h.ok(ws, "commit", "-m", "feat(auth): add token service")
	if !strings.Contains(out, "security-critical-review") {
		t.Fatalf("commit output missing decision: %s", out)
	}

	rangeSpec := base + "..seralith/auth"
	out, _, code := h.run("", "gate", "--range", rangeSpec)
	if code != 1 || !strings.Contains(out, "requires 1 approval(s) from role [security]") {
		t.Fatalf("gate before approval: code=%d\n%s", code, out)
	}

	if _, _, code := h.run("", "approve", "seralith/auth", "--role", "security"); code != 0 {
		t.Fatal("approve command failed")
	}
	out, _, code = h.run("", "gate", "--range", rangeSpec)
	if code != 1 {
		t.Fatalf("self-approval without the security role must not satisfy the gate:\n%s", out)
	}

	h.as("bob")
	bobPub := filepath.Join(t.TempDir(), "bob.pub")
	if err := os.WriteFile(bobPub, []byte(h.ok("", "key", "export")), 0o644); err != nil {
		t.Fatal(err)
	}
	h.as("alice")
	h.ok("", "key", "trust", bobPub, "--principal", "bob@example.com", "--roles", "security")
	h.as("bob")
	h.ok("", "approve", "seralith/auth", "--role", "security", "-m", "token handling reviewed")
	h.as("alice")

	out, _, code = h.run("", "gate", "--range", rangeSpec)
	if code != 0 || !strings.Contains(out, "1 pass") {
		t.Fatalf("gate after approval: code=%d\n%s", code, out)
	}
	md := h.ok("", "gate", "--range", rangeSpec, "--format", "markdown")
	if !strings.Contains(md, "Seralith gate: PASS") {
		t.Fatalf("markdown: %s", md)
	}

	var bl struct {
		Lines []struct {
			Actor struct {
				Kind string `json:"kind"`
				Tool string `json:"tool"`
			} `json:"actor"`
			Verified bool `json:"verified"`
		} `json:"lines"`
	}
	if err := json.Unmarshal([]byte(h.ok("", "--json", "blame", "--rev", "seralith/auth", "internal/auth/token.go")), &bl); err != nil {
		t.Fatal(err)
	}
	if len(bl.Lines) != 3 {
		t.Fatalf("blame lines = %d", len(bl.Lines))
	}
	for _, l := range bl.Lines {
		if l.Actor.Kind != "agent" || l.Actor.Tool != "codex" || !l.Verified {
			t.Fatalf("blame line = %+v", l)
		}
	}
	summary := h.ok("", "blame", "--rev", "seralith/auth", "--summary", "src/app.go")
	if !strings.Contains(summary, "human 1") || !strings.Contains(summary, "unattested 3") {
		t.Fatalf("blame summary = %s", summary)
	}

	pkg := filepath.Join(t.TempDir(), "evidence.zip")
	h.ok("", "evidence", "export", "--range", rangeSpec, "-o", pkg)
	h.ok("", "evidence", "verify", pkg)
	assertZipHas(t, pkg, "manifest.json", "manifest.sig.json", "report.md", "ai-usage.json", "attestations/", "ledger.jsonl")

	h.ok("", "ledger", "checkpoint")
	if out := h.ok("", "ledger", "verify"); !strings.Contains(out, "intact") || !strings.Contains(out, "1/1 checkpoints verified") {
		t.Fatalf("ledger verify: %s", out)
	}
	h.ok("", "ledger", "verify", "--strict")
	if out := h.ok("", "log", "--type", "approval.recorded"); strings.Count(out, "approval.recorded") != 2 {
		t.Fatalf("log: %s", out)
	}
}

func TestPolicyDenyBlocksCommit(t *testing.T) {
	h := newHarness(t)
	h.ok("", "init")
	h.repo.Commit("chore: enable seralith")
	h.ok("", "ws", "new", "ci")
	h.ok("", "run", "--ws", "ci", "--agent", "claude", "--", "sh", "-c", "mkdir -p .github/workflows && echo 'on: push' > .github/workflows/x.yml")
	out, errOut, code := h.run("", "commit", "--ws", "ci", "-m", "ci: tweak")
	if code != 3 || !strings.Contains(errOut, "agents-no-pipeline-edits") {
		t.Fatalf("deny: code=%d out=%s err=%s", code, out, errOut)
	}
	h.ok("", "commit", "--ws", "ci", "-m", "ci: tweak", "--force")
	if out := h.ok("", "log", "--type", "policy.override"); !strings.Contains(out, "policy.override") {
		t.Fatal("override not recorded")
	}
	if _, _, code := h.run("", "verify", "seralith/ci"); code != 1 {
		t.Fatal("verify should fail a denied change even after a forced commit")
	}
}

func TestGateUsesBaseConfiguration(t *testing.T) {
	h := newHarness(t)
	h.ok("", "init")
	base := h.repo.Commit("chore: enable seralith")
	h.ok("", "ws", "new", "relax")
	ws := h.wsPath("relax")
	testutil.WriteFile(t, filepath.Join(ws, ".seralith/policy.yaml"), "version: 1\nrules: []\n")
	h.ok("", "run", "--ws", "relax", "--agent", "codex", "--", "sh", "-c", "mkdir -p .github/workflows && echo 'on: push' > .github/workflows/x.yml")
	h.ok("", "commit", "--ws", "relax", "-m", "ci: relax policy and edit pipeline", "--force")

	h.repo.Git("checkout", "-q", "--detach", "seralith/relax")
	if _, _, code := h.run("", "gate", "--range", base+"..HEAD"); code != 0 {
		t.Fatal("sanity: the relaxed branch policy should pass when trusted")
	}
	out, _, code := h.run("", "gate", "--range", base+"..HEAD", "--config-from", base)
	if code != 1 || !strings.Contains(out, "agents-no-pipeline-edits") {
		t.Fatalf("gate with base config: code=%d\n%s", code, out)
	}
}

func TestTamperedAttestationFailsVerification(t *testing.T) {
	h := newHarness(t)
	h.ok("", "init")
	h.repo.Commit("chore: enable seralith")
	h.ok("", "ws", "new", "w")
	h.ok("", "run", "--ws", "w", "--", "sh", "-c", "echo hi > hello.txt")
	h.ok("", "commit", "--ws", "w", "-m", "feat: hello")
	h.ok("", "verify", "seralith/w")

	other, err := keys.Generate()
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "mallory.pem")
	if err := other.Save(keyPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERALITH_KEY", keyPath)
	h.ok("", "ws", "new", "w2", "--base", "seralith/w")
	h.ok("", "run", "--ws", "w2", "--", "sh", "-c", "echo evil > hello.txt")
	h.ok("", "commit", "--ws", "w2", "-m", "fix: innocent")
	out, _, code := h.run("", "verify", "seralith/w2")
	if code != 1 || !strings.Contains(out, "not trusted") {
		t.Fatalf("untrusted signer: code=%d out=%s", code, out)
	}

	h.repo.Git("checkout", "-q", "-b", "sneaky", "seralith/w")
	h.repo.Write("hello.txt", "replaced\n")
	h.repo.Commit("chore: unattested change")
	out, _, code = h.run("", "verify", "HEAD")
	if code != 1 || !strings.Contains(out, "no change attestation") {
		t.Fatalf("unattested: code=%d out=%s", code, out)
	}
}

func assertZipHas(t *testing.T, path string, names ...string) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, want := range names {
		found := false
		for _, f := range zr.File {
			if f.Name == want || (strings.HasSuffix(want, "/") && strings.HasPrefix(f.Name, want)) {
				found = true
			}
		}
		if !found {
			t.Errorf("evidence package missing %s", want)
		}
	}
}
