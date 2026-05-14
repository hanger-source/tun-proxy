package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type App struct {
	SubscribeURL string `json:"subscribe_url"`
	PACPath      string `json:"pac_path"`
	Nodes        []Node `json:"nodes"`
	SelectedNode int    `json:"selected_node"`
	Connected    bool   `json:"-"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) ConfigDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tun-proxy")
	os.MkdirAll(dir, 0755)
	return dir
}

func (a *App) ConfigPath() string {
	return filepath.Join(a.ConfigDir(), "config.json")
}

func (a *App) SingBoxConfigPath() string {
	return filepath.Join(a.ConfigDir(), "singbox.json")
}

func (a *App) SingBoxBinary() string {
	// Look for sing-box in known locations
	paths := []string{
		filepath.Join(a.ConfigDir(), "sing-box"),
		os.ExpandEnv("$HOME/sing-box-test/sing-box"),
		"/usr/local/bin/sing-box",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "sing-box"
}

func (a *App) LoadConfig() {
	data, err := os.ReadFile(a.ConfigPath())
	if err != nil {
		logInfo("no existing config, starting fresh")
		return
	}
	json.Unmarshal(data, a)
	logInfo("config loaded: %d nodes, subscribe_url=%s", len(a.Nodes), a.SubscribeURL)
}

func (a *App) SaveConfig() {
	data, _ := json.MarshalIndent(a, "", "  ")
	os.WriteFile(a.ConfigPath(), data, 0644)
}

func (a *App) UpdateSubscription() error {
	if a.SubscribeURL == "" {
		return fmt.Errorf("未设置订阅链接")
	}
	logInfo("updating subscription: %s", a.SubscribeURL)
	nodes, err := ParseSubscription(a.SubscribeURL)
	if err != nil {
		logError("subscription parse failed: %v", err)
		return err
	}
	if len(nodes) == 0 {
		logWarn("subscription returned 0 nodes")
		return fmt.Errorf("未解析到节点")
	}
	a.Nodes = nodes
	a.SelectedNode = 0
	// Prefer first vmess node as default (SS nodes may be unreliable)
	for i, n := range nodes {
		if n.Type == "vmess" {
			a.SelectedNode = i
			break
		}
	}
	a.SaveConfig()
	logInfo("subscription updated: %d nodes, default: [%d] %s", len(nodes), a.SelectedNode, nodes[a.SelectedNode].Name)
	for i, n := range nodes {
		logInfo("  [%d] %s (%s) %s:%d", i, n.Name, n.Type, n.Server, n.Port)
	}
	return nil
}

func (a *App) Connect() error {
	if a.Connected {
		a.Disconnect()
	}
	if len(a.Nodes) == 0 {
		return fmt.Errorf("无可用节点")
	}

	// Kill any existing sing-box first (no password needed for this)
	a.killSingBox()

	// Try selected node
	node := a.Nodes[a.SelectedNode]
	logInfo("connecting via node: %s (%s:%d)", node.Name, node.Server, node.Port)

	err := a.startSingBox(a.SelectedNode)
	if err != nil {
		logError("start failed: %v", err)
		return err
	}

	// Verify connectivity
	if !a.verifyConnection() {
		logWarn("connectivity check failed")
		a.killSingBox()
		return fmt.Errorf("连接失败: 无法验证连通性")
	}

	a.Connected = true
	logInfo("connected successfully via %s", node.Name)
	return nil
}

func (a *App) startSingBox(nodeIdx int) error {
	a.killSingBox()

	excludeIPs := resolveServerIPs(a.Nodes)
	logInfo("resolved exclude IPs: %v", excludeIPs)

	var pacRules *PACRules
	if a.PACPath != "" {
		pacRules = GetPACRules(a.PACPath)
	}

	config := GenerateSingBoxConfig(a.Nodes, nodeIdx, excludeIPs, pacRules)
	configData, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(a.SingBoxConfigPath(), configData, 0644)
	logInfo("sing-box config written to %s", a.SingBoxConfigPath())

	binary := a.SingBoxBinary()
	logInfo("sing-box binary: %s", binary)

	// Send start command to helper daemon
	resp, err := sendHelperCommand(HelperRequest{
		Action:     "start",
		BinaryPath: binary,
		ConfigPath: a.SingBoxConfigPath(),
		LogPath:    a.SingBoxLogPath(),
	})
	if err != nil {
		logError("helper command failed: %v", err)
		return fmt.Errorf("helper 通信失败: %v", err)
	}
	if !resp.OK {
		logError("helper start failed: %s", resp.Message)
		return fmt.Errorf("启动失败: %s", resp.Message)
	}

	logInfo("sing-box started via helper")
	time.Sleep(1 * time.Second)
	return nil
}

func (a *App) verifyConnection() bool {
	// Quick TCP connectivity test through TUN
	conn, err := net.DialTimeout("tcp", "8.8.8.8:443", 3*time.Second)
	if err != nil {
		logWarn("connectivity check failed: %v", err)
		return false
	}
	conn.Close()
	logInfo("connectivity check passed")
	return true
}

func (a *App) killSingBox() {
	resp, err := sendHelperCommand(HelperRequest{Action: "stop"})
	if err != nil {
		logWarn("helper stop failed: %v, trying pkill", err)
		exec.Command("pkill", "-f", "sing-box run").Run()
		return
	}
	if !resp.OK {
		logWarn("helper stop: %s", resp.Message)
	}
}

func (a *App) Disconnect() {
	logInfo("disconnecting...")
	a.killSingBox()
	a.Connected = false
	logInfo("disconnected")
}

func (a *App) SingBoxLogPath() string {
	return filepath.Join(a.ConfigDir(), "singbox.log")
}

func (a *App) OpenLog() {
	exec.Command("open", "-a", "Console", a.SingBoxLogPath()).Run()
}

func resolveServerIPs(nodes []Node) []string {
	seen := map[string]bool{}
	var ips []string
	for _, n := range nodes {
		if net.ParseIP(n.Server) != nil {
			if !seen[n.Server] {
				ips = append(ips, n.Server+"/32")
				seen[n.Server] = true
			}
			continue
		}
		addrs, err := net.LookupHost(n.Server)
		if err == nil {
			for _, addr := range addrs {
				if !seen[addr] {
					ips = append(ips, addr+"/32")
					seen[addr] = true
				}
			}
		}
	}
	return ips
}

func (a *App) TestRoute(domain string) string {
	if a.PACPath != "" {
		rules := GetPACRules(a.PACPath)
		if rules != nil {
			for _, d := range rules.ProxyDomains {
				if domain == d || strings.HasSuffix(domain, "."+d) {
					return fmt.Sprintf("[PROXY] %s → 代理 (PAC 黑名单)", domain)
				}
			}
			for _, d := range rules.DirectDomains {
				suffix := d // already has leading dot
				if strings.HasSuffix(domain, suffix) || "."+domain == suffix {
					return fmt.Sprintf("[DIRECT] %s → 直连 (PAC 白名单)", domain)
				}
			}
		}
	}

	// Check built-in rules
	if strings.HasSuffix(domain, ".cn") {
		return fmt.Sprintf("[DIRECT] %s → 直连 (.cn)", domain)
	}

	return fmt.Sprintf("[PROXY] %s → 代理 (默认规则)", domain)
}
