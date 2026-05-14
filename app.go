package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"tun-proxy/internal/config"
	"tun-proxy/internal/engine"
	"tun-proxy/internal/rules"
	"tun-proxy/internal/singbox"
	"tun-proxy/internal/subscription"
)

type App struct {
	Cfg    *config.Config
	Engine *engine.Engine
}

func NewApp() *App {
	cfg, err := config.Load()
	if err != nil {
		logError("config load error: %v", err)
	}
	logInfo("config loaded: %d nodes, subscribe_url=%s", len(cfg.Nodes), cfg.SubscribeURL)
	return &App{Cfg: cfg, Engine: engine.New()}
}

func (a *App) SaveConfig() {
	if err := a.Cfg.Save(); err != nil {
		logError("config save error: %v", err)
	}
}

func (a *App) SingBoxBinary() string {
	binPath, err := ensureSingBox(config.Dir())
	if err != nil {
		logError("ensure sing-box failed: %v", err)
	}
	if binPath != "" {
		return binPath
	}
	for _, p := range []string{filepath.Join(config.Dir(), "sing-box"), "/usr/local/bin/sing-box"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "sing-box"
}

func (a *App) UpdateSubscription() error {
	if a.Cfg.SubscribeURL == "" {
		return fmt.Errorf("未设置订阅链接")
	}
	logInfo("updating subscription: %s", a.Cfg.SubscribeURL)
	nodes, err := subscription.Fetch(a.Cfg.SubscribeURL)
	if err != nil {
		logError("subscription parse failed: %v", err)
		return err
	}
	if len(nodes) == 0 {
		return fmt.Errorf("未解析到节点")
	}
	a.Cfg.Nodes = nodes
	a.Cfg.SelectedNode = 0
	for i, n := range nodes {
		if n.Type == "vmess" {
			a.Cfg.SelectedNode = i
			break
		}
	}
	a.SaveConfig()
	logInfo("subscription updated: %d nodes, default: [%d] %s", len(nodes), a.Cfg.SelectedNode, nodes[a.Cfg.SelectedNode].Name)
	return nil
}

func (a *App) Connect() error {
	if a.Engine.Connected {
		a.Disconnect()
	}
	if len(a.Cfg.Nodes) == 0 {
		return fmt.Errorf("无可用节点")
	}

	node := a.Cfg.Nodes[a.Cfg.SelectedNode]
	logInfo("connecting via node: %s (%s:%d)", node.Name, node.Server, node.Port)

	excludeIPs := singbox.ResolveServerIPs(a.Cfg.Nodes)
	logInfo("resolved exclude IPs: %v", excludeIPs)

	var ruleSet *rules.Rules
	rulesDir := a.Cfg.RulesDir
	if rulesDir == "" {
		rulesDir = config.Dir()
	}
	ruleSet = rules.GetRules(rulesDir)

	sbConfig := singbox.GenerateConfig(a.Cfg.Nodes, a.Cfg.SelectedNode, excludeIPs, ruleSet)
	configPath := filepath.Join(config.Dir(), "singbox.json")
	data, err := json.MarshalIndent(sbConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("config marshal failed: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("config write failed: %v", err)
	}

	binary := a.SingBoxBinary()
	logPath := filepath.Join(config.Dir(), "tun-proxy.log")

	if err := a.Engine.Start(context.Background(), a.Cfg, binary, configPath, logPath); err != nil {
		logError("connect failed: %v", err)
		return fmt.Errorf("连接失败: %v", err)
	}

	logInfo("connected successfully via %s", node.Name)
	return nil
}

func (a *App) Disconnect() {
	logInfo("disconnecting...")
	a.Engine.Stop()
	logInfo("disconnected")
}

func (a *App) OpenLog() {
	logPath := filepath.Join(config.Dir(), "tun-proxy.log")
	if err := exec.Command("open", "-a", "Console", logPath).Run(); err != nil {
		logError("open log failed: %v", err)
	}
}

func (a *App) TestRoute(domain string) string {
	rulesDir := a.Cfg.RulesDir
	if rulesDir == "" {
		rulesDir = config.Dir()
	}
	rset := rules.GetRules(rulesDir)
	if rset != nil {
		for _, d := range rset.ProxyDomains {
			if domain == d || strings.HasSuffix(domain, "."+d) {
				return fmt.Sprintf("[PROXY] %s → 代理 (规则匹配)", domain)
			}
		}
		for _, d := range rset.DirectDomains {
			if strings.HasSuffix(domain, d) || "."+domain == d {
				return fmt.Sprintf("[DIRECT] %s → 直连 (规则匹配)", domain)
			}
		}
	}
	if strings.HasSuffix(domain, ".cn") {
		return fmt.Sprintf("[DIRECT] %s → 直连 (.cn)", domain)
	}
	return fmt.Sprintf("[PROXY] %s → 代理 (默认规则)", domain)
}
