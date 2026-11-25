package dsse_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/pkg/dsse"
)

type resolver map[string]dsse.Verifier

func (r resolver) Resolve(id string) (dsse.Verifier, bool) {
	v, ok := r[id]
	return v, ok
}

func mustKey(t *testing.T) *keys.PrivateKey {
	t.Helper()
	k, err := keys.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestPAE(t *testing.T) {
	got := string(dsse.PAE("http://example.com/HelloWorld", []byte("hello world")))
	want := "DSSEv1 29 http://example.com/HelloWorld 11 hello world"
	if got != want {
		t.Fatalf("PAE = %q, want %q", got, want)
	}
}

func TestSignVerify(t *testing.T) {
	a, b := mustKey(t), mustKey(t)
	env, err := dsse.Sign(dsse.PayloadTypeInToto, []byte(`{"x":1}`), a, b)
	if err != nil {
		t.Fatal(err)
	}
	r := resolver{a.KeyID(): a.Public(), b.KeyID(): b.Public()}
	res, err := env.Verify(r, 2)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(res.Accepted) != 2 {
		t.Fatalf("accepted %d signatures, want 2", len(res.Accepted))
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	k := mustKey(t)
	env, err := dsse.Sign(dsse.PayloadTypeInToto, []byte(`{"x":1}`), k)
	if err != nil {
		t.Fatal(err)
	}
	env.Payload = base64.StdEncoding.EncodeToString([]byte(`{"x":2}`))
	if _, err := env.Verify(resolver{k.KeyID(): k.Public()}, 1); !errors.Is(err, dsse.ErrThresholdNotMet) {
		t.Fatalf("err = %v, want ErrThresholdNotMet", err)
	}
}

func TestVerifyRejectsPayloadTypeSwap(t *testing.T) {
	k := mustKey(t)
	env, err := dsse.Sign(dsse.PayloadTypeInToto, []byte(`{}`), k)
	if err != nil {
		t.Fatal(err)
	}
	env.PayloadType = "application/json"
	if _, err := env.Verify(resolver{k.KeyID(): k.Public()}, 1); err == nil {
		t.Fatal("expected verification failure after payload type swap")
	}
}

func TestVerifyUntrustedKey(t *testing.T) {
	k := mustKey(t)
	env, _ := dsse.Sign(dsse.PayloadTypeInToto, []byte(`{}`), k)
	res, err := env.Verify(resolver{}, 1)
	if !errors.Is(err, dsse.ErrThresholdNotMet) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := res.Rejected[k.KeyID()]; !ok {
		t.Fatal("expected untrusted key in rejected set")
	}
}

func TestDuplicateSignaturesCountOnce(t *testing.T) {
	k := mustKey(t)
	env, _ := dsse.Sign(dsse.PayloadTypeInToto, []byte(`{}`), k)
	env.Signatures = append(env.Signatures, env.Signatures[0])
	if _, err := env.Verify(resolver{k.KeyID(): k.Public()}, 2); !errors.Is(err, dsse.ErrThresholdNotMet) {
		t.Fatalf("duplicate signature satisfied threshold: %v", err)
	}
	if err := env.AddSignature(k); !errors.Is(err, dsse.ErrDuplicateSigner) {
		t.Fatalf("AddSignature duplicate err = %v", err)
	}
}

func TestParseRoundTrip(t *testing.T) {
	k := mustKey(t)
	env, _ := dsse.Sign(dsse.PayloadTypeInToto, []byte(`{"a":"b"}`), k)
	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := dsse.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parsed.Verify(resolver{k.KeyID(): k.Public()}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := dsse.Parse([]byte(`{"payloadType":""}`)); !errors.Is(err, dsse.ErrInvalidEnvelope) {
		t.Fatalf("Parse invalid err = %v", err)
	}
}
