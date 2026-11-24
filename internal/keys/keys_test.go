package keys

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSignVerify(t *testing.T) {
	k, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(k.KeyID(), "ed25519:") || len(k.KeyID()) != len("ed25519:")+32 {
		t.Fatalf("unexpected key id %q", k.KeyID())
	}
	sig, err := k.Sign([]byte("msg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Public().Verify([]byte("msg"), sig); err != nil {
		t.Fatal(err)
	}
	if err := k.Public().Verify([]byte("other"), sig); err == nil {
		t.Fatal("verified signature over different message")
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "key.pem")
	k, created, err := LoadOrCreate(path)
	if err != nil || !created {
		t.Fatalf("LoadOrCreate: created=%v err=%v", created, err)
	}
	again, created, err := LoadOrCreate(path)
	if err != nil || created {
		t.Fatalf("second LoadOrCreate: created=%v err=%v", created, err)
	}
	if again.KeyID() != k.KeyID() {
		t.Fatal("reloaded key has different id")
	}
}

func TestPublicPEMRoundTrip(t *testing.T) {
	k, _ := Generate()
	text, err := k.Public().PEM()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublicPEM([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if pub.KeyID() != k.KeyID() {
		t.Fatal("public key id changed across PEM round trip")
	}
	if _, err := ParsePublicPEM([]byte("garbage")); err == nil {
		t.Fatal("parsed garbage as a public key")
	}
}

func TestTrustStore(t *testing.T) {
	alice, _ := Generate()
	ci, _ := Generate()
	ts := NewTrustStore()
	if _, err := ts.Add(alice.Public(), "alice@example.com", KindHuman, "security", "maintainer", "security"); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Add(ci.Public(), "ci@example.com", KindMachine); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trust.yaml")
	if err := ts.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTrustStore(path)
	if err != nil {
		t.Fatal(err)
	}
	tk, ok := loaded.Lookup(alice.KeyID())
	if !ok || !tk.HasRole("security") || len(tk.Roles) != 2 {
		t.Fatalf("alice entry = %+v", tk)
	}
	if _, ok := loaded.Resolve(ci.KeyID()); !ok {
		t.Fatal("ci key not resolvable")
	}
	loaded.Revoke(ci.KeyID())
	if _, ok := loaded.Resolve(ci.KeyID()); ok {
		t.Fatal("revoked key still resolvable")
	}
}

func TestTrustStoreRejectsMismatchedID(t *testing.T) {
	k, _ := Generate()
	pemText, _ := k.Public().PEM()
	doc := "version: 1\nkeys:\n  - id: ed25519:deadbeef\n    principal: x\n    kind: human\n    publicKey: |\n" + indent(pemText, "      ")
	if _, err := ParseTrustStore([]byte(doc)); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("err = %v, want id mismatch", err)
	}
}

func TestLoadMissingTrustStore(t *testing.T) {
	ts, err := LoadTrustStore(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil || len(ts.Keys) != 0 {
		t.Fatalf("ts=%v err=%v", ts, err)
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n") + "\n"
}
