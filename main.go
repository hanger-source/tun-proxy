package main

import (
	"fmt"
	"os"

	"tun-proxy/internal/rules"

	"github.com/getlantern/systray"
)

func main() {
	initLogger()
	logInfo("tun-proxy starting")
	defer func() {
		if r := recover(); r != nil {
			logError("PANIC: %v", r)
		}
	}()
	systray.Run(onReady, onExit)
}

// Shared state for node menu management
var (
	nodeItems    []*systray.MenuItem
	nodeCancelCh chan struct{}
)

func setupNodeListeners(app *App, mStatus *systray.MenuItem) {
	if nodeCancelCh != nil {
		close(nodeCancelCh)
	}
	nodeCancelCh = make(chan struct{})

	for i, item := range nodeItems {
		go func(idx int, menuItem *systray.MenuItem, cancel chan struct{}) {
			for {
				select {
				case <-cancel:
					return
				case <-menuItem.ClickedCh:
					app.Cfg.SelectedNode = idx
					for j, it := range nodeItems {
						if j == idx {
							it.Check()
						} else {
							it.Uncheck()
						}
					}
					app.SaveConfig()
					if app.Engine.Connected {
						mStatus.SetTitle("切换中...")
						app.Disconnect()
						err := app.Connect()
						if err != nil {
							mStatus.SetTitle("[ERR] " + err.Error())
						} else {
							mStatus.SetTitle("[ON] " + app.Cfg.Nodes[app.Cfg.SelectedNode].Name)
							showAlert("已连接: " + app.Cfg.Nodes[app.Cfg.SelectedNode].Name)
						}
					}
				}
			}
		}(i, item, nodeCancelCh)
	}
}

func rebuildNodeMenu(app *App, mNodes *systray.MenuItem, mStatus *systray.MenuItem) {
	for _, item := range nodeItems {
		item.Hide()
	}
	nodeItems = nil

	for i, n := range app.Cfg.Nodes {
		item := mNodes.AddSubMenuItemCheckbox(n.Name, n.Server, i == app.Cfg.SelectedNode)
		nodeItems = append(nodeItems, item)
	}

	setupNodeListeners(app, mStatus)
}

func onReady() {
	systray.SetTemplateIcon(iconOff, iconOff)
	systray.SetTitle("")
	systray.SetTooltip("TUN Proxy")

	app := NewApp()

	if err := installHelperIfNeeded(); err != nil {
		logError("helper install failed: %v", err)
	}

	mStatus := systray.AddMenuItem("[OFF] 已断开", "")
	mStatus.Disable()
	systray.AddSeparator()

	mConnect := systray.AddMenuItem("连接", "启动 TUN 代理")
	mDisconnect := systray.AddMenuItem("断开", "停止 TUN 代理")
	mDisconnect.Hide()
	systray.AddSeparator()

	mNodes := systray.AddMenuItem("节点", "选择代理节点")
	for i, n := range app.Cfg.Nodes {
		item := mNodes.AddSubMenuItemCheckbox(n.Name, n.Server, i == app.Cfg.SelectedNode)
		nodeItems = append(nodeItems, item)
	}
	setupNodeListeners(app, mStatus)

	systray.AddSeparator()
	mSubscribe := systray.AddMenuItem("更新订阅", "拉取最新节点")
	mSetURL := systray.AddMenuItem("设置订阅链接...", "")
	mSetPAC := systray.AddMenuItem("重新加载规则", "重新读取 ruleset-proxy.json / ruleset-direct.json")
	mTestRoute := systray.AddMenuItem("测试域名路由...", "输入域名查看走代理还是直连")
	mAutoStart := systray.AddMenuItemCheckbox("开机启动", "", isAutoStartEnabled())
	mViewLog := systray.AddMenuItem("查看路由日志", "打开控制台查看连接记录")
	systray.AddSeparator()
	mVersion := systray.AddMenuItem("TunProxy v"+Version, "")
	mVersion.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "")

	// Auto-connect on startup
	if len(app.Cfg.Nodes) > 0 {
		go func() {
			mStatus.SetTitle("连接中...")
			err := app.Connect()
			if err != nil {
				mStatus.SetTitle("[ERR] " + err.Error())
			} else {
				mStatus.SetTitle("[ON] " + app.Cfg.Nodes[app.Cfg.SelectedNode].Name)
				systray.SetTemplateIcon(iconOn, iconOn)
				showAlert("已连接: " + app.Cfg.Nodes[app.Cfg.SelectedNode].Name)
				mConnect.Hide()
				mDisconnect.Show()
			}
		}()
	}

	go func() {
		for {
			select {
			case <-mConnect.ClickedCh:
				if len(app.Cfg.Nodes) == 0 {
					mStatus.SetTitle("[ERR] 无节点，请先设置订阅")
					continue
				}
				mStatus.SetTitle("连接中...")
				err := app.Connect()
				if err != nil {
					mStatus.SetTitle("[ERR] " + err.Error())
					continue
				}
				mStatus.SetTitle("[ON] " + app.Cfg.Nodes[app.Cfg.SelectedNode].Name)
				showAlert("已连接: " + app.Cfg.Nodes[app.Cfg.SelectedNode].Name)
				systray.SetTemplateIcon(iconOn, iconOn)
				mConnect.Hide()
				mDisconnect.Show()

			case <-mDisconnect.ClickedCh:
				app.Disconnect()
				mStatus.SetTitle("[OFF] 已断开")
				systray.SetTemplateIcon(iconOff, iconOff)
				mDisconnect.Hide()
				mConnect.Show()

			case <-mSubscribe.ClickedCh:
				mStatus.SetTitle("更新订阅中...")
				prevName := ""
				if app.Cfg.SelectedNode < len(app.Cfg.Nodes) {
					prevName = app.Cfg.Nodes[app.Cfg.SelectedNode].Name
				}
				err := app.UpdateSubscription()
				if err != nil {
					mStatus.SetTitle("[ERR] " + err.Error())
					continue
				}
				if prevName != "" {
					for i, n := range app.Cfg.Nodes {
						if n.Name == prevName {
							app.Cfg.SelectedNode = i
							break
						}
					}
				}
				app.SaveConfig()
				rebuildNodeMenu(app, mNodes, mStatus)
				mStatus.SetTitle(fmt.Sprintf("已更新 %d 个节点", len(app.Cfg.Nodes)))

			case <-mSetURL.ClickedCh:
				url := promptInput("输入订阅链接（完整 URL）", app.Cfg.SubscribeURL)
				if url != "" {
					app.Cfg.SubscribeURL = url
					app.SaveConfig()
					mStatus.SetTitle("订阅链接已保存，请点击「更新订阅」")
					showAlert("订阅链接已保存")
				}

			case <-mSetPAC.ClickedCh:
				// Rules are now JSON files in ~/.tun-proxy/ directory
				// Just clear cache and reconnect to pick up any changes
				rules.ClearCache()
				if app.Engine.Connected {
					app.Disconnect()
					if err := app.Connect(); err != nil {
						mStatus.SetTitle("[ERR] " + err.Error())
					} else {
						mStatus.SetTitle("[ON] " + app.Cfg.Nodes[app.Cfg.SelectedNode].Name)
					}
				}
				showAlert("规则已重新加载")

			case <-mTestRoute.ClickedCh:
				domain := promptInput("输入域名测试路由（如 google.com）", "")
				if domain != "" {
					result := app.TestRoute(domain)
					showAlert(result)
					mStatus.SetTitle(result)
				}

			case <-mAutoStart.ClickedCh:
				if mAutoStart.Checked() {
					disableAutoStart()
					mAutoStart.Uncheck()
					showAlert("已关闭开机启动")
				} else {
					if err := enableAutoStart(); err != nil {
						showAlert("设置失败: " + err.Error())
					} else {
						mAutoStart.Check()
						showAlert("已开启开机启动")
					}
				}

			case <-mViewLog.ClickedCh:
				app.OpenLog()

			case <-mQuit.ClickedCh:
				app.Disconnect()
				systray.Quit()
				os.Exit(0)
			}
		}
	}()
}

func onExit() {}
