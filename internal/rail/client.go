// Package rail is a small client for the RAIL HTTP API.
package rail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotFound is returned when RAIL has no link with the requested id.
var ErrNotFound = errors.New("link not found")

// Client talks to one RAIL tier.
type Client struct {
	endpoint *url.URL
	http     *http.Client
}

// New returns a client for the RAIL tier at endpoint, e.g. "http://edge.rail.localhost:8080".
func New(endpoint string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid RAIL endpoint %q: %w", endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("RAIL endpoint must be an http(s) URL, got %q", endpoint)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialLocalhostFallback
	return &Client{
		endpoint: u,
		// Shortening fans out through every nested tier and vendor; be patient.
		http: &http.Client{Timeout: 2 * time.Minute, Transport: transport},
	}, nil
}

// dialLocalhostFallback dials normally, then retries `*.localhost` names on loopback. RFC 6761
// says they are loopback, browsers and curl agree, and macOS's resolver does not: the reference
// docker compose Pyramid lives at edge.rail.localhost.
func dialLocalhostFallback(ctx context.Context, network, addr string) (net.Conn, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, network, addr)
	host, port, splitErr := net.SplitHostPort(addr)
	var dnsErr *net.DNSError
	if err == nil || splitErr != nil || !errors.As(err, &dnsErr) {
		return conn, err
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host != "localhost" && !strings.HasSuffix(host, ".localhost") {
		return nil, err
	}
	return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
}

// Mirror is one vendor's copy of a link.
type Mirror struct {
	Upstream    string  `json:"upstream"`
	ShortURL    string  `json:"short_url"`
	Healthy     bool    `json:"healthy"`
	LatencyMs   *int64  `json:"latency_ms"`
	LastChecked *int64  `json:"last_checked"`
	LastError   *string `json:"last_error"`
}

// Failure is an upstream that could not mint a mirror.
type Failure struct {
	Upstream string `json:"upstream"`
	Error    string `json:"error"`
}

// Link is RAIL's view of a shortened URL.
type Link struct {
	ID               string    `json:"id"`
	ShortURL         string    `json:"short_url"`
	Target           string    `json:"target"`
	CreatedAt        int64     `json:"created_at"`
	Degraded         bool      `json:"degraded"`
	CompressionRatio float64   `json:"compression_ratio"`
	Mirrors          []Mirror  `json:"mirrors"`
	Failed           []Failure `json:"failed"`
}

// Topology is a tier and its direct upstreams.
type Topology struct {
	ID        string   `json:"id"`
	Instance  string   `json:"instance"`
	PublicURL string   `json:"public_url"`
	Upstreams []string `json:"upstreams"`
}

// Hop is one step of a redirect chain.
type Hop struct {
	URL       string  `json:"url"`
	Status    *int64  `json:"status"`
	LatencyMs int64   `json:"latency_ms"`
	Instance  *string `json:"instance"`
	Error     *string `json:"error"`
}

// Trace is a link's full redirect chain as seen from the tier.
type Trace struct {
	Hops      []Hop `json:"hops"`
	Redirects int64 `json:"redirects"`
	Headroom  int64 `json:"headroom"`
	GaveUp    bool  `json:"gave_up"`
}

// Shorten provisions a link for target, or returns the existing one.
func (c *Client) Shorten(ctx context.Context, target string) (*Link, error) {
	body, err := json.Marshal(map[string]string{"url": target})
	if err != nil {
		return nil, err
	}
	var link Link
	return &link, c.do(ctx, http.MethodPost, "api/shorten", body, &link)
}

// Link fetches a link by id.
func (c *Client) Link(ctx context.Context, id string) (*Link, error) {
	var link Link
	return &link, c.do(ctx, http.MethodGet, url.PathEscape(id)+"/info", nil, &link)
}

// Topology fetches the tier's topology.
func (c *Client) Topology(ctx context.Context) (*Topology, error) {
	var t Topology
	return &t, c.do(ctx, http.MethodGet, "api/topology", nil, &t)
}

// Trace walks a link's redirect chain from the tier.
func (c *Client) Trace(ctx context.Context, id string) (*Trace, error) {
	var t Trace
	return &t, c.do(ctx, http.MethodGet, url.PathEscape(id)+"/trace", nil, &t)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	ref, err := url.Parse(path)
	if err != nil {
		return err
	}
	base := *c.endpoint
	if base.Path == "" || base.Path[len(base.Path)-1] != '/' {
		base.Path += "/"
	}
	req, err := http.NewRequestWithContext(ctx, method, base.ResolveReference(ref).String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "terraform-provider-rail")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("RAIL is unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 300 {
		var problem struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &problem) == nil && problem.Error != "" {
			return fmt.Errorf("RAIL refused (%d): %s", resp.StatusCode, problem.Error)
		}
		return fmt.Errorf("RAIL refused (%d): %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("RAIL answered with unexpected JSON: %w", err)
	}
	return nil
}
