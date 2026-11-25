// Package cloud is a client for the Seralith Control API.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryan-tobin/new-startup/internal/fsutil"
	"github.com/ryan-tobin/new-startup/pkg/dsse"
)

type Credentials struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// LoadCredentials reads saved credentials, letting SERALITH_CLOUD_URL and SERALITH_CLOUD_TOKEN override them.
func LoadCredentials(path string) (*Credentials, error) {
	c := &Credentials{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("cloud: parse %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if v := os.Getenv("SERALITH_CLOUD_URL"); v != "" {
		c.URL = v
	}
	if v := os.Getenv("SERALITH_CLOUD_TOKEN"); v != "" {
		c.Token = v
	}
	if c.URL == "" || c.Token == "" {
		return nil, errors.New("cloud: not logged in; run `seralith cloud login` or set SERALITH_CLOUD_URL and SERALITH_CLOUD_TOKEN")
	}
	return c, nil
}

func SaveCredentials(path string, c *Credentials) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fsutil.WriteAtomic(path, data, 0o600)
}

type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

func New(c *Credentials) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(c.URL, "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("cloud: invalid URL %q", c.URL)
	}
	if u.Scheme == "http" && !isLocal(u.Hostname()) {
		return nil, errors.New("cloud: refusing to send credentials over plain HTTP to a non-local host")
	}
	return &Client{base: u, token: c.Token, http: &http.Client{Timeout: 60 * time.Second}}, nil
}

func isLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("control API %d: %s", e.Status, e.Message) }

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base.String()+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", "seralith-cli")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = http.StatusText(resp.StatusCode)
		}
		return &APIError{Status: resp.StatusCode, Message: e.Error}
	}
	if out == nil {
		return nil
	}
	if raw, ok := out.(*[]byte); ok {
		*raw = data
		return nil
	}
	return json.Unmarshal(data, out)
}

type Me struct {
	User struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"user"`
	Org struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
		Plan string `json:"plan"`
	} `json:"org"`
}

func (c *Client) Me(ctx context.Context) (*Me, error) {
	var me Me
	return &me, c.do(ctx, http.MethodGet, "/v1/me", nil, &me)
}

type PushResult struct {
	Stored    int `json:"stored"`
	Untrusted int `json:"untrusted"`
}

// Push uploads envelopes in batches.
func (c *Client) Push(ctx context.Context, project string, envs []*dsse.Envelope) (*PushResult, error) {
	total := &PushResult{}
	for start := 0; start < len(envs); start += 500 {
		end := min(start+500, len(envs))
		var res PushResult
		if err := c.do(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(project)+"/attestations", map[string]any{"envelopes": envs[start:end]}, &res); err != nil {
			return nil, err
		}
		total.Stored += res.Stored
		total.Untrusted += res.Untrusted
	}
	return total, nil
}

func (c *Client) TrustYAML(ctx context.Context) ([]byte, error) {
	var data []byte
	return data, c.do(ctx, http.MethodGet, "/v1/trust.yaml", nil, &data)
}

func (c *Client) PolicyYAML(ctx context.Context, project string) ([]byte, error) {
	var data []byte
	return data, c.do(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(project)+"/policy.yaml", nil, &data)
}

func (c *Client) Stats(ctx context.Context, project string) (map[string]any, error) {
	out := map[string]any{}
	return out, c.do(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(project)+"/stats", nil, &out)
}
