package rail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/prefix/api/shorten":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["url"] == "http://127.0.0.1/" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"127.0.0.1 is not a public address"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"abc","short_url":"http://rail/abc","target":"` + body["url"] + `","mirrors":[{"upstream":"is.gd","short_url":"https://is.gd/x","healthy":true}]}`))
		case r.URL.Path == "/prefix/abc/info":
			_, _ = w.Write([]byte(`{"id":"abc","target":"https://example.com/","mirrors":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// The endpoint's path prefix is kept even without a trailing slash.
	c, err := New(srv.URL + "/prefix")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	link, err := c.Shorten(ctx, "https://example.com/")
	if err != nil || link.ID != "abc" || len(link.Mirrors) != 1 {
		t.Fatalf("Shorten = %+v, %v", link, err)
	}
	if _, err := c.Shorten(ctx, "http://127.0.0.1/"); err == nil || !strings.Contains(err.Error(), "not a public address") {
		t.Fatalf("expected RAIL's error message, got %v", err)
	}
	if _, err := c.Link(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Link(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestNewRejectsNonHTTP(t *testing.T) {
	for _, endpoint := range []string{"ftp://rail", "rail.example.com", ""} {
		if _, err := New(endpoint); err == nil {
			t.Errorf("New(%q) should fail", endpoint)
		}
	}
}

func TestLocalhostSubdomainsFallBackToLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"x","instance":"edge"}`))
	}))
	defer srv.Close()
	_, port, _ := strings.Cut(srv.URL, "127.0.0.1:")
	c, err := New("http://edge.rail-test.localhost:" + port)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := c.Topology(context.Background())
	if err != nil || topology.Instance != "edge" {
		t.Fatalf("Topology = %+v, %v", topology, err)
	}
}
