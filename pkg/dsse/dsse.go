// Package dsse implements the Dead Simple Signing Envelope (DSSE) v1 protocol.
package dsse

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// PayloadTypeInToto is the payload type for in-toto statements.
const PayloadTypeInToto = "application/vnd.in-toto+json"

var (
	ErrNoSignatures     = errors.New("dsse: envelope has no signatures")
	ErrThresholdNotMet  = errors.New("dsse: signature threshold not met")
	ErrInvalidEnvelope  = errors.New("dsse: invalid envelope")
	ErrDuplicateSigner  = errors.New("dsse: signer already present")
	ErrPayloadTypeEmpty = errors.New("dsse: payload type is empty")
)

type Signature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

type Envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"`
	Signatures  []Signature `json:"signatures"`
}

type Signer interface {
	KeyID() string
	Sign(message []byte) ([]byte, error)
}

type Verifier interface {
	KeyID() string
	Verify(message, signature []byte) error
}

// KeyResolver maps a key ID to a verifier. Implementations decide which keys are trusted.
type KeyResolver interface {
	Resolve(keyID string) (Verifier, bool)
}

// PAE computes the DSSE pre-authentication encoding.
func PAE(payloadType string, payload []byte) []byte {
	buf := make([]byte, 0, len(payloadType)+len(payload)+32)
	buf = append(buf, "DSSEv1 "...)
	buf = strconv.AppendInt(buf, int64(len(payloadType)), 10)
	buf = append(buf, ' ')
	buf = append(buf, payloadType...)
	buf = append(buf, ' ')
	buf = strconv.AppendInt(buf, int64(len(payload)), 10)
	buf = append(buf, ' ')
	buf = append(buf, payload...)
	return buf
}

// Sign creates an envelope over payload signed by every signer.
func Sign(payloadType string, payload []byte, signers ...Signer) (*Envelope, error) {
	if payloadType == "" {
		return nil, ErrPayloadTypeEmpty
	}
	env := &Envelope{
		PayloadType: payloadType,
		Payload:     base64.StdEncoding.EncodeToString(payload),
		Signatures:  []Signature{},
	}
	for _, s := range signers {
		if err := env.AddSignature(s); err != nil {
			return nil, err
		}
	}
	return env, nil
}

// AddSignature appends a signature from s, rejecting duplicate key IDs.
func (e *Envelope) AddSignature(s Signer) error {
	for _, existing := range e.Signatures {
		if existing.KeyID == s.KeyID() {
			return fmt.Errorf("%w: %s", ErrDuplicateSigner, s.KeyID())
		}
	}
	payload, err := e.DecodePayload()
	if err != nil {
		return err
	}
	sig, err := s.Sign(PAE(e.PayloadType, payload))
	if err != nil {
		return fmt.Errorf("dsse: sign with %s: %w", s.KeyID(), err)
	}
	e.Signatures = append(e.Signatures, Signature{
		KeyID: s.KeyID(),
		Sig:   base64.StdEncoding.EncodeToString(sig),
	})
	return nil
}

// DecodePayload returns the raw payload bytes.
func (e *Envelope) DecodePayload() ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("%w: payload is not base64: %w", ErrInvalidEnvelope, err)
	}
	return b, nil
}

// VerificationResult describes which signatures on an envelope were accepted.
type VerificationResult struct {
	Accepted []string
	Rejected map[string]error
}

// Verify checks signatures against keys known to r and requires at least threshold
// distinct trusted keys to have produced valid signatures.
func (e *Envelope) Verify(r KeyResolver, threshold int) (*VerificationResult, error) {
	if threshold < 1 {
		threshold = 1
	}
	if e.PayloadType == "" {
		return nil, ErrPayloadTypeEmpty
	}
	if len(e.Signatures) == 0 {
		return nil, ErrNoSignatures
	}
	payload, err := e.DecodePayload()
	if err != nil {
		return nil, err
	}
	msg := PAE(e.PayloadType, payload)
	res := &VerificationResult{Rejected: map[string]error{}}
	seen := map[string]bool{}
	for _, s := range e.Signatures {
		if seen[s.KeyID] {
			continue
		}
		v, ok := r.Resolve(s.KeyID)
		if !ok {
			res.Rejected[s.KeyID] = errors.New("untrusted key")
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(s.Sig)
		if err != nil {
			res.Rejected[s.KeyID] = fmt.Errorf("signature is not base64: %w", err)
			continue
		}
		if err := v.Verify(msg, raw); err != nil {
			res.Rejected[s.KeyID] = err
			continue
		}
		seen[s.KeyID] = true
		res.Accepted = append(res.Accepted, s.KeyID)
	}
	if len(res.Accepted) < threshold {
		return res, fmt.Errorf("%w: %d of %d", ErrThresholdNotMet, len(res.Accepted), threshold)
	}
	return res, nil
}

// Marshal encodes the envelope as compact JSON.
func (e *Envelope) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

// Parse decodes a JSON envelope.
func Parse(data []byte) (*Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidEnvelope, err)
	}
	if e.PayloadType == "" || e.Payload == "" {
		return nil, ErrInvalidEnvelope
	}
	return &e, nil
}
