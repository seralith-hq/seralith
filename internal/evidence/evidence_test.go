package evidence

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/testutil"
	"github.com/ryan-tobin/new-startup/internal/verify"
)

func build(t *testing.T) (string, *keys.TrustStore) {
	t.Helper()
	g := testutil.NewGitRepo(t)
	g.Write("a", "a\n")
	g.Commit("init")
	repo, _ := gitx.Open(g.Dir)
	k, _ := keys.Generate()
	ts := keys.NewTrustStore()
	if _, err := ts.Add(k.Public(), "dev@example.com", keys.KindHuman); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, err := Build(context.Background(), Input{
		Repo: repo, Range: "HEAD~0..HEAD", Gate: &verify.GateReport{Status: verify.StatusPass, Lines: map[string]int{}},
		PolicyFile: []byte("version: 1\nrules: []\n"), TrustFile: []byte("version: 1\n"), Signer: k, Project: "demo", Now: time.Unix(1_800_000_000, 0).UTC(),
	}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "e.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, ts
}

func TestBuildAndVerify(t *testing.T) {
	path, ts := build(t)
	res, err := Verify(path, ts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || len(res.SignedBy) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if res2, _ := Verify(path, keys.NewTrustStore()); res2.OK() {
		t.Fatal("package verified against an empty trust store")
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	path, ts := build(t)
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if f.Name == "policy.yaml" {
			data = []byte("version: 1\nrules: []\n# relaxed\n")
		}
		w, _ := zw.Create(f.Name)
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	_ = zr.Close()
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Verify(path, ts)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() || len(res.Mismatch) != 1 || res.Mismatch[0] != "policy.yaml" {
		t.Fatalf("result = %+v", res)
	}
}
