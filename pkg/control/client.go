// Package control is the agent's HTTP client for talking to the
// saas/server control plane: fetching nonces, enrolling, heartbeating, and
// resolving connect-info for a target peer. Every authenticated request is
// signed with the agent's libp2p identity key, mirroring the self-signed
// message pattern in pkg/discovery.
package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client

	priv   crypto.PrivKey
	peerID string
	pubB64 string
}

func New(baseURL string, priv crypto.PrivKey) (*Client, error) {
	pub := priv.GetPublic()

	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("failed to derive peer id: %w", err)
	}

	pubBytes, err := crypto.MarshalPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		priv:    priv,
		peerID:  id.String(),
		pubB64:  base64.StdEncoding.EncodeToString(pubBytes),
	}, nil
}

func (c *Client) PeerID() string { return c.peerID }

type nonceResponse struct {
	Nonce    string `json:"nonce"`
	IssuedAt string `json:"issued_at"`
}

// Nonce fetches a fresh single-use nonce to bind the next signed request.
func (c *Client) Nonce(ctx context.Context) (string, error) {
	var resp nonceResponse
	if err := c.do(ctx, http.MethodGet, "/api/peers/nonce", nil, &resp); err != nil {
		return "", err
	}
	return resp.Nonce, nil
}

func (c *Client) sign(payload any) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal signed payload: %w", err)
	}

	sig, err := c.priv.Sign(b)
	if err != nil {
		return "", fmt.Errorf("failed to sign payload: %w", err)
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}

// Action discriminators included in every signed payload so a signature
// produced for one endpoint can't be replayed against another: without
// this, enrollSignedPayload and meSignedPayload were structurally
// identical ({ID, Nonce, Timestamp}), making their signed bytes
// interchangeable.
const (
	actionEnroll      = "enroll"
	actionMe          = "me"
	actionHeartbeat   = "heartbeat"
	actionConnectInfo = "connect-info"
)

type enrollSignedPayload struct {
	Action    string `json:"action"`
	ID        string `json:"id"`
	PublicKey string `json:"public_key"`
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
}

type EnrollResult struct {
	ID         string   `json:"id"`
	Role       string   `json:"role"`
	RelayAddrs []string `json:"relay_addrs"`
}

// Enroll redeems a one-time enrollment token, registering this agent's
// libp2p identity as a peer under the token's owning account.
func (c *Client) Enroll(ctx context.Context, token string) (*EnrollResult, error) {
	nonce, err := c.Nonce(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nonce: %w", err)
	}
	ts := time.Now().Unix()

	sig, err := c.sign(enrollSignedPayload{Action: actionEnroll, ID: c.peerID, PublicKey: c.pubB64, Nonce: nonce, Timestamp: ts})
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"token":      token,
		"id":         c.peerID,
		"public_key": c.pubB64,
		"nonce":      nonce,
		"timestamp":  ts,
		"signature":  sig,
	}

	var result EnrollResult
	if err := c.do(ctx, http.MethodPost, "/api/peers/enroll", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type meSignedPayload struct {
	Action    string `json:"action"`
	ID        string `json:"id"`
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
}

// NodeSummary identifies one exit node available to a client, as reported
// by the control plane.
type NodeSummary struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// MeResult is this agent's own enrollment info, as currently known by the
// control plane.
type MeResult struct {
	ID         string   `json:"id"`
	Role       string   `json:"role"`
	RelayAddrs []string `json:"relay_addrs"`
	// Nodes is populated only when Role == "client": every exit node on
	// the same account, fetched fresh on every call. A client should use
	// this instead of relying on locally configured target node IDs.
	Nodes []NodeSummary `json:"nodes,omitempty"`
}

// Me fetches this agent's own role and the current control-plane relay
// address. Nothing from the result is meant to be cached to disk: callers
// should call this on every startup (the persistent identity key is the
// only thing that needs to survive a restart) and keep the result in
// memory only.
func (c *Client) Me(ctx context.Context) (*MeResult, error) {
	nonce, err := c.Nonce(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nonce: %w", err)
	}
	ts := time.Now().Unix()

	sig, err := c.sign(meSignedPayload{Action: actionMe, ID: c.peerID, Nonce: nonce, Timestamp: ts})
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("id", c.peerID)
	q.Set("nonce", nonce)
	q.Set("timestamp", strconv.FormatInt(ts, 10))
	q.Set("signature", sig)

	var result MeResult
	if err := c.do(ctx, http.MethodGet, "/api/peers/me?"+q.Encode(), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type heartbeatSignedPayload struct {
	Action    string   `json:"action"`
	ID        string   `json:"id"`
	Nonce     string   `json:"nonce"`
	Timestamp int64    `json:"timestamp"`
	Addrs     []string `json:"addrs,omitempty"`
}

// Heartbeat reports liveness and the agent's currently reachable addresses.
func (c *Client) Heartbeat(ctx context.Context, addrs []string) error {
	nonce, err := c.Nonce(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch nonce: %w", err)
	}
	ts := time.Now().Unix()

	sig, err := c.sign(heartbeatSignedPayload{Action: actionHeartbeat, ID: c.peerID, Nonce: nonce, Timestamp: ts, Addrs: addrs})
	if err != nil {
		return err
	}

	body := map[string]any{
		"id":        c.peerID,
		"nonce":     nonce,
		"timestamp": ts,
		"addrs":     addrs,
		"signature": sig,
	}

	return c.do(ctx, http.MethodPost, "/api/peers/heartbeat", body, nil)
}

type connectInfoSignedPayload struct {
	Action      string `json:"action"`
	RequesterID string `json:"requester_id"`
	TargetID    string `json:"target_id"`
	Nonce       string `json:"nonce"`
	Timestamp   int64  `json:"timestamp"`
}

type ConnectInfo struct {
	ID                     string   `json:"id"`
	Addrs                  []string `json:"addrs"`
	RelayAddrs             []string `json:"relay_addrs"`
	RelayReservationNeeded bool     `json:"relay_reservation_required"`
}

// ConnectInfo resolves a target peer's last-known addresses and the
// control-plane relay address, for direct-then-relay dialing.
func (c *Client) ConnectInfo(ctx context.Context, targetID string) (*ConnectInfo, error) {
	nonce, err := c.Nonce(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nonce: %w", err)
	}
	ts := time.Now().Unix()

	sig, err := c.sign(connectInfoSignedPayload{
		Action: actionConnectInfo, RequesterID: c.peerID, TargetID: targetID, Nonce: nonce, Timestamp: ts,
	})
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("requester_id", c.peerID)
	q.Set("nonce", nonce)
	q.Set("timestamp", strconv.FormatInt(ts, 10))
	q.Set("signature", sig)

	path := "/api/peers/" + url.PathEscape(targetID) + "/connect-info?" + q.Encode()

	var info ConnectInfo
	if err := c.do(ctx, http.MethodGet, path, nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

type errorResponse struct {
	Error string `json:"error"`
}

// APIError is returned by Client methods for any non-2xx control-plane
// response, carrying the actual HTTP status so callers can react to
// specific conditions (e.g. 401 meaning this identity's peer record is
// gone/revoked) instead of string-matching an error message.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("control plane returned %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("control plane returned status %d", e.StatusCode)
}

// maxRateLimitRetries bounds how many times do will honor a 429's
// Retry-After and try again, so a persistently rate-limited caller fails
// loudly instead of blocking (near-)indefinitely.
const maxRateLimitRetries = 3

// defaultRetryAfter is used when a 429 response is missing or has an
// unparseable Retry-After header — still backs off rather than
// immediately hammering the endpoint again.
const defaultRetryAfter = 2 * time.Second

// maxRetryAfter caps how long do will ever wait on a single Retry-After,
// against a misconfigured or malicious server sending an absurd value.
const maxRetryAfter = 60 * time.Second

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyBytes = b
	}

	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
		if err != nil {
			return fmt.Errorf("failed to build request: %w", err)
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("request to %s failed: %w", path, err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRateLimitRetries {
			wait := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()

			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
				continue
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}

		if resp.StatusCode >= 300 {
			var errResp errorResponse
			_ = json.NewDecoder(resp.Body).Decode(&errResp)
			resp.Body.Close()
			return &APIError{StatusCode: resp.StatusCode, Message: errResp.Error}
		}

		if out != nil {
			err := json.NewDecoder(resp.Body).Decode(out)
			resp.Body.Close()
			if err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}
			return nil
		}

		resp.Body.Close()
		return nil
	}
}

// parseRetryAfter interprets an HTTP Retry-After header value, which per
// RFC 9110 §10.2.3 is either a number of seconds or an HTTP-date. Falls
// back to defaultRetryAfter if empty or unparseable, and caps the result
// at maxRetryAfter.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return defaultRetryAfter
	}

	// A header that parses but names a non-positive duration (e.g. "0", or
	// an HTTP-date already in the past) is a deliberate "retry now"
	// signal, distinct from a missing/unparseable header — which instead
	// falls back to defaultRetryAfter below, since it carries no
	// information at all.
	if seconds, err := strconv.Atoi(header); err == nil {
		return min(max(time.Duration(seconds)*time.Second, 0), maxRetryAfter)
	}

	if when, err := http.ParseTime(header); err == nil {
		return min(max(time.Until(when), 0), maxRetryAfter)
	}

	return defaultRetryAfter
}
