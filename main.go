package main

import (
	"fmt"
	"os"

	"github.com/getlantern/systray"
)

func main() {
	initLogger()
	logInfo("tun-proxy starting")
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(iconOff)
	systray.SetTitle("")
	systray.SetTooltip("TUN Proxy")

	app := NewApp()
	app.LoadConfig()

	mStatus := systray.AddMenuItem("[OFF] 已断开", "")
	mStatus.Disable()
	systray.AddSeparator()

	mConnect := systray.AddMenuItem("连接", "启动 TUN 代理")
	mDisconnect := systray.AddMenuItem("断开", "停止 TUN 代理")
	mDisconnect.Hide()
	systray.AddSeparator()

	// Node selection submenu
	mNodes := systray.AddMenuItem("节点", "选择代理节点")
	var nodeItems []*systray.MenuItem
	for i, n := range app.Nodes {
		item := mNodes.AddSubMenuItemCheckbox(n.Name, n.Server, i == app.SelectedNode)
		nodeItems = append(nodeItems, item)
	}

	systray.AddSeparator()
	mSubscribe := systray.AddMenuItem("更新订阅", "拉取最新节点")
	mSetURL := systray.AddMenuItem("设置订阅链接...", "")
	mSetPAC := systray.AddMenuItem("设置 PAC 文件路径...", "白名单域名直连")
	mTestRoute := systray.AddMenuItem("测试域名路由...", "输入域名查看走代理还是直连")
	mAutoStart := systray.AddMenuItemCheckbox("开机启动", "", isAutoStartEnabled())
	mViewLog := systray.AddMenuItem("查看路由日志", "打开控制台查看连接记录")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "")

	go func() {
		for {
			select {
			case <-mConnect.ClickedCh:
				if len(app.Nodes) == 0 {
					mStatus.SetTitle("[ERR] 无节点，请先设置订阅")
					continue
				}
				mStatus.SetTitle("连接中...")
				err := app.Connect()
				if err != nil {
					mStatus.SetTitle("[ERR] " + err.Error())
					continue
				}
				mStatus.SetTitle("[ON] " + app.Nodes[app.SelectedNode].Name)
				systray.SetIcon(iconOn)
				mConnect.Hide()
				mDisconnect.Show()

			case <-mDisconnect.ClickedCh:
				app.Disconnect()
				mStatus.SetTitle("[OFF] 已断开")
				systray.SetIcon(iconOff)
				mDisconnect.Hide()
				mConnect.Show()

			case <-mSubscribe.ClickedCh:
				mStatus.SetTitle("更新订阅中...")
				err := app.UpdateSubscription()
				if err != nil {
					mStatus.SetTitle("[ERR] " + err.Error())
					continue
				}
				// Rebuild node menu
				for _, item := range nodeItems {
					item.Hide()
				}
				nodeItems = nil
				for i, n := range app.Nodes {
					item := mNodes.AddSubMenuItemCheckbox(n.Name, n.Server, i == 0)
					nodeItems = append(nodeItems, item)
				}
				app.SelectedNode = 0
				app.SaveConfig()
				mStatus.SetTitle(fmt.Sprintf("已更新 %d 个节点", len(app.Nodes)))

			case <-mSetURL.ClickedCh:
				url := promptInput("输入订阅链接（完整 URL）", app.SubscribeURL)
				if url != "" {
					app.SubscribeURL = url
					app.SaveConfig()
					mStatus.SetTitle("订阅链接已保存，请点击「更新订阅」")
					showAlert("订阅链接已保存")
				}

			case <-mSetPAC.ClickedCh:
				path := promptFileChooser("选择 PAC 文件")
				if path != "" {
					app.PACPath = path
					ClearPACCache()
					app.SaveConfig()
					mStatus.SetTitle("PAC: " + path)
					showAlert("PAC 文件已设置")
					if app.Connected {
						app.Disconnect()
						app.Connect()
						mStatus.SetTitle("[ON] " + app.Nodes[app.SelectedNode].Name + " (PAC)")
					}
				}

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
					err := enableAutoStart()
					if err != nil {
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

	// Handle node selection - launch goroutine per node
	for i, item := range nodeItems {
		go func(idx int, menuItem *systray.MenuItem) {
			for range menuItem.ClickedCh {
				app.SelectedNode = idx
				for j, it := range nodeItems {
					if j == idx {
						it.Check()
					} else {
						it.Uncheck()
					}
				}
				app.SaveConfig()
				if app.Connected {
					mStatus.SetTitle("切换中...")
					app.Disconnect()
					err := app.Connect()
					if err != nil {
						mStatus.SetTitle("[ERR] " + err.Error())
					} else {
						mStatus.SetTitle("[ON] " + app.Nodes[app.SelectedNode].Name)
					}
				}
			}
		}(i, item)
	}
}

func onExit() {}
