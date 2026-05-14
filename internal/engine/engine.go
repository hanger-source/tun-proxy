package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
	"tun-proxy/internal/config"
)

const helperSockPath = "/var/run/tun-proxy.sock"

type HelperRequest struct {
	Action     string `json:"action"`
	ConfigPath string `json:"config_path,omitempty"`
	BinaryPath string `json:"binary_path,omitempty"`
	LogPath    string `json:"log_path,omitempty"`
}

type HelperResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type Engine struct {
	Connected bool
	cancel    context.CancelFunc
}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) Start(ctx context.Context, cfg *config.Config, singboxBin, configPath, logPath string) error {
	e.Stop()

	resp, err := sendCommand(HelperRequest{
		Action:     "start",
		BinaryPath: singboxBin,
		ConfigPath: configPath,
		LogPath:    logPath,
	})
	if err != nil {
		return fmt.Errorf("helper communication failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("start failed: %s", resp.Message)
	}

	// Wait for TUN to be ready
	time.Sleep(1 * time.Second)

	// Verify connectivity
	if err := e.verify(ctx); err != nil {
		e.Stop()
		return fmt.Errorf("connectivity check failed: %w", err)
	}

	e.Connected = true
	return nil
}

func (e *Engine) Stop() {
	sendCommand(HelperRequest{Action: "stop"})
	e.Connected = false
}

func (e *Engine) verify(ctx context.Context) error {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", "8.8.8.8:443")
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func sendCommand(req HelperRequest) (*HelperResponse, error) {
	conn, err := net.DialTimeout("unix", helperSockPath, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to helper: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("send failed: %w", err)
	}
	var resp HelperResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("recv failed: %w", err)
	}
	return &resp, nil
}
