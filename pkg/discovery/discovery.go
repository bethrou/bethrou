package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/henrybarreto/bethrou/pkg/config"
	"github.com/henrybarreto/bethrou/pkg/logging"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/redis/go-redis/v9"
)

// Request represents a discovery request message
type Request struct {
	Action string `json:"action"`
	Replay string `json:"reply,omitempty"`
}

// Response represents a discovery response message
type Response struct {
	ID    string   `json:"id"`
	Addrs []string `json:"addrs"`
}

// Config contains configuration for the discovery service
type Config struct {
	Address string
	Topic   string
	Timeout time.Duration
	User    string
	Pass    string
}

// Service handles discovery operations using Redis pub/sub
type Service struct {
	config Config
	host   host.Host
	client *redis.Client
}

// NewService creates a new discovery service
func NewService(cfg Config, h host.Host) (*Service, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("discovery address is required")
	}

	opt, err := redis.ParseURL(cfg.Address)
	if err != nil {
		opt = &redis.Options{Addr: cfg.Address}
	}

	if cfg.User != "" {
		opt.Username = cfg.User
	}
	if cfg.Pass != "" {
		opt.Password = cfg.Pass
	}

	client := redis.NewClient(opt)

	return &Service{
		config: cfg,
		host:   h, // Can be nil for client-only mode
		client: client,
	}, nil
}

// Discover sends a discovery request and collects responses from nodes
func (s *Service) Discover(ctx context.Context) ([]config.NodeConfig, error) {
	replay := fmt.Sprintf("client-reply-%d", time.Now().UnixNano())

	pubsub := s.client.Subscribe(ctx, replay)
	defer func() {
		if err := pubsub.Close(); err != nil {
			logging.Logger.Warn("warning: pubsub close error", "error", err)
		}
	}()

	if _, err := pubsub.Receive(ctx); err != nil {
		return nil, fmt.Errorf("failed to subscribe to reply topic: %w", err)
	}

	if err := s.publishDiscovery(ctx, replay); err != nil {
		return nil, err
	}

	return s.collectDiscover(ctx, pubsub)
}

// Start starts the discovery service in server mode (responds to requests)
func (s *Service) Start(ctx context.Context) error {
	if s.host == nil {
		return fmt.Errorf("host is required for server mode")
	}

	topic := s.config.Topic
	if topic == "" {
		topic = s.host.ID().String()
	}

	pubsub := s.client.Subscribe(ctx, topic)
	defer pubsub.Close()

	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}

	ch := pubsub.Channel()
	logging.Logger.Info("Subscribed to discovery topic", "topic", topic)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}

			logging.Logger.Debug("Received discovery message", "payload", msg.Payload)

			if err := s.process(ctx, msg.Payload); err != nil {
				logging.Logger.Error("error processing discovery message", "error", err)
			}
		}
	}
}

// Close closes the discovery service
func (s *Service) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// publishDiscovery publishes a discovery request to the discovery topic
func (s *Service) publishDiscovery(ctx context.Context, replay string) error {
	req := Request{
		Action: "discover",
		Replay: replay,
	}

	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	if err := s.client.Publish(ctx, s.config.Topic, string(b)).Err(); err != nil {
		return fmt.Errorf("failed to publish discovery request: %w", err)
	}

	return nil
}

// collectDiscover collects discovery responses from nodes
func (s *Service) collectDiscover(ctx context.Context, pubsub *redis.PubSub) ([]config.NodeConfig, error) {
	ch := pubsub.Channel()
	timeout := s.config.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	discoveredMap := make(map[string]config.NodeConfig)

LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		case <-deadline.C:
			break LOOP
		case msg := <-ch:
			if msg == nil {
				break LOOP
			}

			var resp Response
			if err := json.Unmarshal([]byte(msg.Payload), &resp); err != nil {
				return nil, fmt.Errorf("invalid JSON: %w", err)
			}

			if resp.ID == "" || len(resp.Addrs) == 0 {
				return nil, fmt.Errorf("incomplete response: missing ID or addresses")
			}

			node := &config.NodeConfig{
				ID:    resp.ID,
				Addrs: resp.Addrs,
			}

			if node != nil && node.ID != "" {
				if _, exists := discoveredMap[node.ID]; !exists {
					discoveredMap[node.ID] = *node
				}
			}
		}
	}

	discoveredSlice := make([]config.NodeConfig, 0, len(discoveredMap))
	for _, n := range discoveredMap {
		discoveredSlice = append(discoveredSlice, n)
	}

	return discoveredSlice, nil
}

// process processes an incoming discovery request and publishes node info
func (s *Service) process(ctx context.Context, payload string) error {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}

	var req Request
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		logging.Logger.Warn("discovery: ignoring non-json payload", "payload", payload)
		return nil
	}

	reply := req.Replay
	if reply == "" {
		logging.Logger.Warn("discovery: no reply topic in message, ignoring", "req", req)
		return nil
	}

	if req.Action != "" && req.Action != "discover" {
		logging.Logger.Debug("discovery: ignoring message with action", "action", req.Action)
		return nil
	}

	return s.publish(ctx, reply)
}

// publish publishes this node's information to a reply topic
func (s *Service) publish(ctx context.Context, reply string) error {
	addrs := make([]string, 0, len(s.host.Addrs()))
	for _, a := range s.host.Addrs() {
		addrs = append(addrs, a.String()+"/p2p/"+s.host.ID().String())
	}

	resp := Response{
		ID:    s.host.ID().String(),
		Addrs: addrs,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	if err := s.client.Publish(ctx, reply, string(b)).Err(); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", reply, err)
	}

	logging.Logger.Info("discovery: published node info", "reply", reply)

	return nil
}
