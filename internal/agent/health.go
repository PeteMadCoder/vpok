package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// HealthStatus represents the current health state
type HealthStatus struct {
	Healthy          bool
	ConsecutiveFails int
	LastCheck        time.Time
	LastError        error
}

// HealthChecker periodically verifies service health.
type HealthChecker struct {
	config  *HealthCheckConfig
	client  *http.Client
	status  HealthStatus
	onState func(healthy bool, err error)
}

// NewHealthChecker creates a HealthChecker instance.
func NewHealthChecker(cfg *HealthCheckConfig, onStateChange func(healthy bool, err error)) *HealthChecker {
	timeout := 2 * time.Second
	if cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil {
			timeout = d
		}
	}

	return &HealthChecker{
		config: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
		status: HealthStatus{
			Healthy: true,
		},
		onState: onStateChange,
	}
}

// CheckOnce executes a single health probe.
func (h *HealthChecker) CheckOnce(ctx context.Context) error {
	if h.config == nil {
		return nil
	}

	switch h.config.Type {
	case "http":
		url := fmt.Sprintf("http://127.0.0.1:%d%s", h.config.Port, h.config.Path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := h.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			return fmt.Errorf("health check returned non-2xx status: %d", resp.StatusCode)
		}
		return nil

	case "tcp":
		address := fmt.Sprintf("127.0.0.1:%d", h.config.Port)
		d := net.Dialer{Timeout: h.client.Timeout}
		conn, err := d.DialContext(ctx, "tcp", address)
		if err != nil {
			return err
		}
		conn.Close()
		return nil

	default:
		return fmt.Errorf("unsupported health check type: %s", h.config.Type)
	}
}

// Start runs the periodic health check loop until ctx is canceled.
func (h *HealthChecker) Start(ctx context.Context) {
	if h.config == nil {
		return
	}

	interval := 10 * time.Second
	if h.config.Interval != "" {
		if d, err := time.ParseDuration(h.config.Interval); err == nil {
			interval = d
		}
	}

	maxFailures := h.config.Failures
	if maxFailures <= 0 {
		maxFailures = 3
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := h.CheckOnce(ctx)
			h.status.LastCheck = time.Now()
			h.status.LastError = err

			if err != nil {
				h.status.ConsecutiveFails++
				if h.status.ConsecutiveFails >= maxFailures && h.status.Healthy {
					h.status.Healthy = false
					if h.onState != nil {
						h.onState(false, err)
					}
				} else {
					if !h.status.Healthy || h.status.ConsecutiveFails > 0 {
						h.status.Healthy = true
						h.status.ConsecutiveFails = 0
						if h.onState != nil {
							h.onState(true, nil)
						}
					}
				}
			}
		}
	}
}
