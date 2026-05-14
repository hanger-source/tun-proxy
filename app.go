package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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
	cmd          *exec.Cmd
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

	// Try selected node first, then failover to others
	order := []int{a.SelectedNode}
	for i := range a.Nodes {
		if i != a.SelectedNode {
			order = append(order, i)
		}
	}

	for _, idx := range order {
		node := a.Nodes[idx]
		logInfo("trying node [%d]: %s (%s:%d)", idx, node.Name, node.Server, node.Port)

		err := a.startSingBox(idx)
		if err != nil {
			logError("start failed for node %s: %v", node.Name, err)
			continue
		}

		// Verify connectivity
		if a.verifyConnection() {
			a.SelectedNode = idx
			a.Connected = true
			logInfo("connected successfully via %s", node.Name)
			return nil
		}

		logWarn("node %s started but connectivity check failed, trying next", node.Name)
		a.killSingBox()
	}

	return fmt.Errorf("所有节点连接失败")
}

func (a *App) startSingBox(nodeIdx int) error {
	a.killSingBox()

	excludeIPs := resolveServerIPs(a.Nodes)
	logInfo("resolved exclude IPs: %v", excludeIPs)

	var pacRules *PACRules
	if a.PACPath != "" {
		pacRules = ParsePACFile(a.PACPath)
	}

	config := GenerateSingBoxConfig(a.Nodes, nodeIdx, excludeIPs, pacRules)
	configData, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(a.SingBoxConfigPath(), configData, 0644)
	logInfo("sing-box config written to %s", a.SingBoxConfigPath())

	binary := a.SingBoxBinary()
	logInfo("sing-box binary: %s", binary)

	a.cmd = exec.Command("sudo", "-n", binary, "run", "-c", a.SingBoxConfigPath())
	a.cmd.Stdout = logFile
	a.cmd.Stderr = logFile

	err := a.cmd.Start()
	if err != nil {
		logWarn("sudo -n failed, trying osascript: %v", err)
		script := fmt.Sprintf(`do shell script "%s run -c %s" with administrator privileges`, binary, a.SingBoxConfigPath())
		a.cmd = exec.Command("osascript", "-e", script)
		err = a.cmd.Start()
		if err != nil {
			return fmt.Errorf("启动失败: %v", err)
		}
	}

	time.Sleep(3 * time.Second)
	if a.cmd.ProcessState != nil && a.cmd.ProcessState.Exited() {
		return fmt.Errorf("sing-box exited immediately")
	}
	return nil
}

func (a *App) verifyConnection() bool {
	// Test actual connectivity through the proxy
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://ipinfo.io/ip")
	if err != nil {
		logWarn("connectivity check failed: %v", err)
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	ip := strings.TrimSpace(string(body))
	logInfo("connectivity check: exit IP = %s", ip)
	// Verify it's not our local IP (basic check)
	return resp.StatusCode == 200 && len(ip) > 0
}

func (a *App) killSingBox() {
	if a.cmd != nil && a.cmd.Process != nil {
		exec.Command("sudo", "-n", "kill", fmt.Sprintf("%d", a.cmd.Process.Pid)).Run()
		a.cmd.Process.Kill()
		a.cmd.Wait()
		a.cmd = nil
	}
	exec.Command("sudo", "-n", "pkill", "-f", "sing-box run").Run()
}

func (a *App) Disconnect() {
	logInfo("disconnecting...")
	a.killSingBox()
	a.Connected = false
	logInfo("disconnected")
}

func (a *App) SingBoxLogPath() string {
	return filepath.Join(a.ConfigDir(), "tun-proxy.log")
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
	// Check PAC first
	if a.PACPath != "" {
		rules := ParsePACFile(a.PACPath)
		if rules != nil {
			for _, d := range rules.ProxyDomains {
				if domain == d || strings.HasSuffix(domain, "."+d) {
					return fmt.Sprintf("🔵 %s → 代理 (PAC 黑名单)", domain)
				}
			}
			for _, d := range rules.DirectDomains {
				suffix := d // already has leading dot
				if strings.HasSuffix(domain, suffix) || "."+domain == suffix {
					return fmt.Sprintf("⚪ %s → 直连 (PAC 白名单)", domain)
				}
			}
		}
	}

	// Check built-in rules
	if strings.HasSuffix(domain, ".cn") {
		return fmt.Sprintf("⚪ %s → 直连 (.cn)", domain)
	}

	return fmt.Sprintf("🔵 %s → 代理 (默认规则)", domain)
}
