package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const sockPath = "/var/run/tun-proxy.sock"
const allowedBinary = "/Users" // Must be under /Users or ~/.tun-proxy

type Request struct {
	Action     string `json:"action"` // start, stop, status
	ConfigPath string `json:"config_path,omitempty"`
	BinaryPath string `json:"binary_path,omitempty"`
	LogPath    string `json:"log_path,omitempty"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

var singboxCmd *exec.Cmd

func main() {
	// Remove stale socket
	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	// Restrict socket access - only root and staff group
	os.Chmod(sockPath, 0660)
	// Set group to staff so the app user can connect
	exec.Command("chgrp", "staff", sockPath).Run()

	fmt.Println("tun-proxy-helper started, listening on", sockPath)

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		stopSingBox()
		os.Remove(sockPath)
		os.Exit(0)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		sendResponse(conn, false, "invalid request")
		return
	}

	switch req.Action {
	case "start":
		if req.BinaryPath == "" || req.ConfigPath == "" {
			sendResponse(conn, false, "missing binary_path or config_path")
			return
		}
		// Security: only allow sing-box binary from trusted paths
		if !isAllowedBinary(req.BinaryPath) {
			sendResponse(conn, false, "binary path not allowed")
			return
		}
		stopSingBox()
		err := startSingBox(req.BinaryPath, req.ConfigPath, req.LogPath)
		if err != nil {
			sendResponse(conn, false, err.Error())
		} else {
			sendResponse(conn, true, "started")
		}

	case "stop":
		stopSingBox()
		sendResponse(conn, true, "stopped")

	case "status":
		running := singboxCmd != nil && singboxCmd.ProcessState == nil
		if running {
			// Check if process is actually alive
			if singboxCmd.Process != nil {
				if err := singboxCmd.Process.Signal(syscall.Signal(0)); err != nil {
					running = false
				}
			}
		}
		if running {
			sendResponse(conn, true, "running")
		} else {
			sendResponse(conn, true, "stopped")
		}

	default:
		sendResponse(conn, false, "unknown action")
	}
}

func startSingBox(binary, config, logPath string) error {
	singboxCmd = exec.Command(binary, "run", "-c", config)

	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			singboxCmd.Stdout = f
			singboxCmd.Stderr = f
			// Ensure file is world-writable so app user can also write
			os.Chmod(logPath, 0666)
		}
	}

	if err := singboxCmd.Start(); err != nil {
		return fmt.Errorf("start failed: %v", err)
	}

	// Monitor process in background
	go func() {
		singboxCmd.Wait()
	}()

	return nil
}

func stopSingBox() {
	if singboxCmd != nil && singboxCmd.Process != nil {
		singboxCmd.Process.Signal(syscall.SIGTERM)
		singboxCmd.Process.Kill()
		singboxCmd.Wait()
		singboxCmd = nil
	}
	// Also kill any orphaned sing-box
	exec.Command("pkill", "-f", "sing-box run").Run()
}

func sendResponse(conn net.Conn, ok bool, msg string) {
	json.NewEncoder(conn).Encode(Response{OK: ok, Message: msg})
}

func isAllowedBinary(path string) bool {
	// Only allow sing-box from known safe locations
	allowed := []string{
		"/.tun-proxy/sing-box",
		"/usr/local/bin/sing-box",
	}
	for _, suffix := range allowed {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
