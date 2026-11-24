// Package keys manages Ed25519 signing keys and the set of keys a repository trusts.
package keys

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	pemPrivateType = "PRIVATE KEY"
	pemPublicType  = "PUBLIC KEY"
)

var ErrInvalidSignature = errors.New("keys: invalid signature")

// ID derives a stable key identifier from an Ed25519 public key.
func ID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "ed25519:" + hex.EncodeToString(sum[:16])
}

type PublicKey struct {
	id  string
	key ed25519.PublicKey
}

func NewPublicKey(pub ed25519.PublicKey) *PublicKey {
	return &PublicKey{id: ID(pub), key: pub}
}

func (p *PublicKey) KeyID() string          { return p.id }
func (p *PublicKey) Raw() ed25519.PublicKey { return p.key }
func (p *PublicKey) Verify(msg, sig []byte) error {
	if !ed25519.Verify(p.key, msg, sig) {
		return ErrInvalidSignature
	}
	return nil
}

// PEM encodes the public key as a PKIX PEM block.
func (p *PublicKey) PEM() (string, error) {
	der, err := x509.MarshalPKIXPublicKey(p.key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: pemPublicType, Bytes: der})), nil
}

type PrivateKey struct {
	pub  *PublicKey
	priv ed25519.PrivateKey
}

// Generate creates a new random Ed25519 key pair.
func Generate() (*PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("keys: generate: %w", err)
	}
	return &PrivateKey{pub: NewPublicKey(pub), priv: priv}, nil
}

func (k *PrivateKey) KeyID() string      { return k.pub.id }
func (k *PrivateKey) Public() *PublicKey { return k.pub }
func (k *PrivateKey) Sign(msg []byte) ([]byte, error) {
	return k.priv.Sign(rand.Reader, msg, crypto.Hash(0))
}

// PEM encodes the private key as a PKCS#8 PEM block.
func (k *PrivateKey) PEM() ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(k.priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: pemPrivateType, Bytes: der}), nil
}

// ParsePrivatePEM decodes a PKCS#8 Ed25519 private key.
func ParsePrivatePEM(data []byte) (*PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != pemPrivateType {
		return nil, errors.New("keys: no private key PEM block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("keys: parse private key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("keys: private key is not Ed25519")
	}
	return &PrivateKey{pub: NewPublicKey(priv.Public().(ed25519.PublicKey)), priv: priv}, nil
}

// ParsePublicPEM decodes a PKIX Ed25519 public key.
func ParsePublicPEM(data []byte) (*PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != pemPublicType {
		return nil, errors.New("keys: no public key PEM block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("keys: parse public key: %w", err)
	}
	pub, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("keys: public key is not Ed25519")
	}
	return NewPublicKey(pub), nil
}

// Save writes the private key to path with owner-only permissions.
func (k *PrivateKey) Save(path string) error {
	data, err := k.PEM()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads a private key from path.
func Load(path string) (*PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParsePrivatePEM(data)
}

// LoadOrCreate loads the key at path, generating and saving one if it does not exist.
func LoadOrCreate(path string) (*PrivateKey, bool, error) {
	k, err := Load(path)
	if err == nil {
		return k, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	k, err = Generate()
	if err != nil {
		return nil, false, err
	}
	if err := k.Save(path); err != nil {
		return nil, false, err
	}
	return k, true, nil
}
