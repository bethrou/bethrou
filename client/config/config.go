package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/henrybarreto/bethrou/pkg/config"
)

type ServerConfig struct {
	ListenAddr string `yaml:"listen"`
	Auth       bool   `yaml:"auth"`
	User       string `yaml:"user,omitempty"`
	Pass       string `yaml:"pass,omitempty"`
}

func (s *ServerConfig) Validate() error {
	if s.ListenAddr == "" {
		return errors.New("SOCKS listen address is required")
	}

	if s.Auth {
		if s.User == "" || s.Pass == "" {
			return errors.New("SOCKS auth enabled but user or pass is empty")
		}
	}

	return nil
}

type RoutingConfig struct {
	Strategy string `yaml:"strategy"`
	Health   string `yaml:"health"`
	Timeout  string `yaml:"timeout"`
}

func (s *RoutingConfig) Validate() error {
	switch s.Strategy {
	case "", "random", "fastest", "round-robin":

	default:
		return fmt.Errorf("unsupported routing strategy: %s", s.Strategy)
	}

	if s.Health != "" {
		if _, err := time.ParseDuration(s.Health); err != nil {
			return fmt.Errorf("invalid routing.health duration: %w", err)
		}
	}

	if s.Timeout != "" {
		if _, err := time.ParseDuration(s.Timeout); err != nil {
			return fmt.Errorf("invalid routing.timeout duration: %w", err)
		}
	}

	return nil
}

type NodeConfig = config.NodeConfig

type DiscoveryConfig = config.DiscoveryConfig

type LogConfig = config.LogConfig

type ClientConfig struct {
	Key       string           `yaml:"key"`
	Server    *ServerConfig    `yaml:"server"`
	Routing   *RoutingConfig   `yaml:"routing"`
	Nodes     []NodeConfig     `yaml:"nodes"`
	Discovery *DiscoveryConfig `yaml:"discovery"`
	Log       *LogConfig       `yaml:"log"`
}

func (c *ClientConfig) Validate() error {
	if c.Key == "" {
		return errors.New("network key is required")
	}

	if c.Server == nil {
		return errors.New("server config is required")
	}

	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}

	if c.Routing == nil {
		return errors.New("routing config is required")
	}

	if err := c.Routing.Validate(); err != nil {
		return fmt.Errorf("routing config validation failed: %w", err)
	}

	if c.Discovery == nil {
		return errors.New("discovery config is required")
	}

	if err := c.Discovery.Validate(); err != nil {
		return fmt.Errorf("discovery config validation failed: %w", err)
	}

	if len(c.Nodes) == 0 && !c.Discovery.Enabled {
		return errors.New("at least one static node or discovery must be enabled")
	}

	if c.Log == nil {
		return errors.New("log config is required")
	}

	if err := c.Log.Validate(); err != nil {
		return fmt.Errorf("log config validation failed: %w", err)
	}

	return nil
}

func (c *ClientConfig) String() string {
	return fmt.Sprintf("ClientConfig{Key: %s, Server: %+v, Routing: %+v, Discovery: %+v, Nodes: %d, Log: %+v}",
		c.Key, c.Server, c.Routing, c.Discovery, len(c.Nodes), c.Log)
}
