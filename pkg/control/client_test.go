package control

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
)

func genIdentity(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	return priv
}

// A 429 with Retry-After: 0 must be retried immediately (not treated as
// "no info", which would apply a slower default backoff) and the retry
// must succeed once the server stops denying.
func TestClientDoRetriesOnRateLimitWithRetryAfter(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"too many requests, try again later"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"nonce":"abc","issued_at":"now"}`))
	}))
	defer ts.Close()

	c, err := New(ts.URL, genIdentity(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	nonce, err := c.Nonce(context.Background())
	if err != nil {
		t.Fatalf("Nonce: %v", err)
	}
	if nonce != "abc" {
		t.Fatalf("expected nonce %q, got %q", "abc", nonce)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected exactly 2 calls (1 denied + 1 retry), got %d", got)
	}
}

// Persistent rate limiting must eventually surface as an error rather than
// retrying forever.
func TestClientDoGivesUpAfterMaxRateLimitRetries(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"too many requests, try again later"}`))
	}))
	defer ts.Close()

	c, err := New(ts.URL, genIdentity(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := c.Nonce(context.Background()); err == nil {
		t.Fatal("expected persistent rate limiting to eventually return an error")
	}

	if got := calls.Load(); got != maxRateLimitRetries+1 {
		t.Fatalf("expected %d calls (initial + %d retries), got %d", maxRateLimitRetries+1, maxRateLimitRetries, got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"empty falls back to default", "", defaultRetryAfter},
		{"unparseable falls back to default", "not-a-number-or-date", defaultRetryAfter},
		{"zero seconds means retry now", "0", 0},
		{"positive seconds", "5", 5 * time.Second},
		{"negative seconds clamps to zero", "-5", 0},
		{"absurd value is capped", strconv.Itoa(int((10 * time.Minute).Seconds())), maxRetryAfter},
		{"past HTTP-date means retry now", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat), 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfter(tc.header)
			if got != tc.want {
				t.Fatalf("parseRetryAfter(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}
