package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// HealthCheckConfig defines in-guest health verification parameters.
type HealthCheckConfig struct {
	Type     string `json:"type"` // http or tcp
	Port     int    `json:"port"`
	Path     string `json:"path"`
	Interval string `json:"interval"`
	Timeout  string `json:"timeout"`
	Failures int    `json:"failures"`
}

// Config represents the complete runtime payload passed to vpok-agent.
type Config struct {
	Entrypoint  []string           `json:"entrypoint"`
	WorkingDir  string             `json:"workingDir,omitempty"`
	Env         map[string]string  `json:"env,omitempty"`
	Secrets     map[string]string  `json:"secrets,omitempty"` // Name -> Content
	GracePeriod string             `json:"gracePeriod,omitempty"`
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`
}

// ParseGracePeriod parses the grace period string into a time.Duration
func (c *Config) ParseGracePeriod() time.Duration {
	if c.GracePeriod == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(c.GracePeriod)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// LoadConfig reads and decodes the agent config from a JSON file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode agent config: %w", err)
	}

	if len(cfg.Entrypoint) == 0 {
		return nil, fmt.Errorf("entrypoint command cannot be empty")
	}

	return &cfg, nil
}
