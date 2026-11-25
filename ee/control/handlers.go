package control

import (
	"net/http"
	"strconv"

	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/internal/policy"
	"github.com/seralith-hq/seralith/pkg/dsse"
)

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Store.Keys(r.Context(), userFrom(r).OrgID)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleAddKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Principal string   `json:"principal"`
		Kind      string   `json:"kind"`
		Roles     []string `json:"roles"`
		PublicKey string   `json:"publicKey"`
	}
	if err := readJSON(r, 1<<16, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Kind == "" {
		req.Kind = string(keys.KindHuman)
	}
	if req.Principal == "" || (req.Kind != string(keys.KindHuman) && req.Kind != string(keys.KindMachine)) {
		writeErr(w, http.StatusBadRequest, "principal and kind (human or machine) are required")
		return
	}
	pub, err := keys.ParsePublicPEM([]byte(req.PublicKey))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u := userFrom(r)
	if req.Roles == nil {
		req.Roles = []string{}
	}
	k := TrustedKey{ID: pub.KeyID(), Principal: req.Principal, Kind: req.Kind, Roles: req.Roles, PublicKey: req.PublicKey, CreatedBy: u.Email}
	if err := s.cfg.Store.PutKey(r.Context(), u.OrgID, k); err != nil {
		storeErr(w, err)
		return
	}
	_ = s.cfg.Store.Audit(r.Context(), u.OrgID, u.Email, "key.trusted", k.ID, map[string]any{"principal": k.Principal, "kind": k.Kind, "roles": k.Roles})
	writeJSON(w, http.StatusCreated, k)
}

func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if err := s.cfg.Store.RevokeKey(r.Context(), u.OrgID, r.PathValue("id")); err != nil {
		storeErr(w, err)
		return
	}
	_ = s.cfg.Store.Audit(r.Context(), u.OrgID, u.Email, "key.revoked", r.PathValue("id"), nil)
	w.WriteHeader(http.StatusNoContent)
}

// handleTrustYAML exports the registry in the .seralith/trust.yaml format so CI gates
// verify against the organization's current keys.
func (s *Server) handleTrustYAML(w http.ResponseWriter, r *http.Request) {
	ts, err := s.cfg.Store.TrustStore(r.Context(), userFrom(r).OrgID)
	if err != nil {
		storeErr(w, err)
		return
	}
	data, err := ts.Marshal()
	if err != nil {
		storeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(data)
}

func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	pv, err := s.cfg.Store.LatestPolicy(r.Context(), p.ID)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

func (s *Server) handlePolicyYAML(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	pv, err := s.cfg.Store.LatestPolicy(r.Context(), p.ID)
	if err != nil {
		storeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("X-Seralith-Policy-Version", strconv.Itoa(pv.Version))
	_, _ = w.Write([]byte(pv.Document))
}

func (s *Server) handlePutPolicy(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	var req struct {
		Document string `json:"document"`
	}
	if err := readJSON(r, 1<<20, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	parsed, err := policy.Parse([]byte(req.Document))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	u := userFrom(r)
	pv, err := s.cfg.Store.PutPolicy(r.Context(), p.ID, req.Document, parsed.Digest(), u.Email)
	if err != nil {
		storeErr(w, err)
		return
	}
	_ = s.cfg.Store.Audit(r.Context(), u.OrgID, u.Email, "policy.published", p.Name, map[string]any{"version": pv.Version, "digest": pv.Digest})
	writeJSON(w, http.StatusCreated, pv)
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	var req struct {
		Envelopes []*dsse.Envelope `json:"envelopes"`
	}
	if err := readJSON(r, 32<<20, &req); err != nil || len(req.Envelopes) == 0 || len(req.Envelopes) > 1000 {
		writeErr(w, http.StatusBadRequest, "envelopes must contain between 1 and 1000 DSSE envelopes")
		return
	}
	results, err := s.cfg.Store.Ingest(r.Context(), p, req.Envelopes)
	if err != nil {
		storeErr(w, err)
		return
	}
	stored, untrusted := 0, 0
	for _, res := range results {
		if res.Stored {
			stored++
		}
		if !res.Verified {
			untrusted++
		}
	}
	s.metrics.ingested.Add(int64(stored))
	u := userFrom(r)
	_ = s.cfg.Store.Audit(r.Context(), u.OrgID, u.Email, "attestations.ingested", p.Name, map[string]int{"received": len(results), "stored": stored, "untrusted": untrusted})
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "stored": stored, "untrusted": untrusted})
}

func (s *Server) handleListAttestations(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	list, err := s.cfg.Store.Attestations(r.Context(), p.ID, AttestationQuery{
		Commit: q.Get("commit"), PredicateType: q.Get("type"), Limit: limit, WithEnvelope: q.Get("envelope") == "true",
	})
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	st, err := s.cfg.Store.Stats(r.Context(), p.ID)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.cfg.Store.AuditEvents(r.Context(), userFrom(r).OrgID, limit)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleAuditVerify(w http.ResponseWriter, r *http.Request) {
	rep, err := s.cfg.Store.VerifyAudit(r.Context(), userFrom(r).OrgID)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
