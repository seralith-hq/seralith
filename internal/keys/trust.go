package keys

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/seralith-hq/seralith/pkg/dsse"
	"gopkg.in/yaml.v3"
)

type PrincipalKind string

const (
	KindHuman   PrincipalKind = "human"
	KindMachine PrincipalKind = "machine"
)

// TrustedKey binds a public key to an accountable principal and the roles they hold.
type TrustedKey struct {
	ID        string        `yaml:"id" json:"id"`
	Principal string        `yaml:"principal" json:"principal"`
	Kind      PrincipalKind `yaml:"kind" json:"kind"`
	Roles     []string      `yaml:"roles,omitempty" json:"roles,omitempty"`
	PublicKey string        `yaml:"publicKey" json:"publicKey"`
	Revoked   bool          `yaml:"revoked,omitempty" json:"revoked,omitempty"`

	parsed *PublicKey
}

func (t *TrustedKey) HasRole(role string) bool {
	return slices.Contains(t.Roles, role)
}

// TrustStore is the set of keys a repository accepts signatures from.
type TrustStore struct {
	Version int           `yaml:"version" json:"version"`
	Keys    []*TrustedKey `yaml:"keys" json:"keys"`
}

func NewTrustStore() *TrustStore {
	return &TrustStore{Version: 1}
}

// LoadTrustStore parses a trust file, returning an empty store if it does not exist.
func LoadTrustStore(path string) (*TrustStore, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewTrustStore(), nil
	}
	if err != nil {
		return nil, err
	}
	return ParseTrustStore(data)
}

func ParseTrustStore(data []byte) (*TrustStore, error) {
	ts := NewTrustStore()
	if err := yaml.Unmarshal(data, ts); err != nil {
		return nil, fmt.Errorf("trust: parse: %w", err)
	}
	if ts.Version != 1 {
		return nil, fmt.Errorf("trust: unsupported version %d", ts.Version)
	}
	for _, k := range ts.Keys {
		pub, err := ParsePublicPEM([]byte(k.PublicKey))
		if err != nil {
			return nil, fmt.Errorf("trust: key for %s: %w", k.Principal, err)
		}
		if pub.KeyID() != k.ID {
			return nil, fmt.Errorf("trust: key id mismatch for %s: declared %s, computed %s", k.Principal, k.ID, pub.KeyID())
		}
		k.parsed = pub
	}
	return ts, nil
}

// Add trusts pub for principal, replacing any existing entry for the same key.
func (ts *TrustStore) Add(pub *PublicKey, principal string, kind PrincipalKind, roles ...string) (*TrustedKey, error) {
	pemText, err := pub.PEM()
	if err != nil {
		return nil, err
	}
	sorted := slices.Clone(roles)
	sort.Strings(sorted)
	tk := &TrustedKey{
		ID:        pub.KeyID(),
		Principal: principal,
		Kind:      kind,
		Roles:     slices.Compact(sorted),
		PublicKey: pemText,
		parsed:    pub,
	}
	for i, existing := range ts.Keys {
		if existing.ID == tk.ID {
			ts.Keys[i] = tk
			return tk, nil
		}
	}
	ts.Keys = append(ts.Keys, tk)
	return tk, nil
}

// Revoke marks a key as no longer trusted for new verifications.
func (ts *TrustStore) Revoke(id string) bool {
	for _, k := range ts.Keys {
		if k.ID == id {
			k.Revoked = true
			return true
		}
	}
	return false
}

// Lookup returns the trust entry for id, including revoked keys.
func (ts *TrustStore) Lookup(id string) (*TrustedKey, bool) {
	for _, k := range ts.Keys {
		if k.ID == id {
			return k, true
		}
	}
	return nil, false
}

// Resolve implements dsse.KeyResolver, ignoring revoked keys.
func (ts *TrustStore) Resolve(id string) (dsse.Verifier, bool) {
	k, ok := ts.Lookup(id)
	if !ok || k.Revoked || k.parsed == nil {
		return nil, false
	}
	return k.parsed, true
}

func (ts *TrustStore) Marshal() ([]byte, error) {
	return yaml.Marshal(ts)
}

func (ts *TrustStore) Save(path string) error {
	data, err := ts.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
