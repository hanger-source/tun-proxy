package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

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

func sendHelperCommand(req HelperRequest) (*HelperResponse, error) {
	conn, err := net.DialTimeout("unix", helperSockPath, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to helper: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("send failed: %v", err)
	}

	var resp HelperResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("recv failed: %v", err)
	}

	return &resp, nil
}
