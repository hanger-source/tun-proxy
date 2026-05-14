# TunProxy

macOS 菜单栏代理工具，通过 TUN 虚拟网卡实现真正的全局代理。

## 为什么需要这个

macOS 上的系统代理（HTTP/SOCKS）只对主动读取代理设置的应用生效。很多应用（如 Electron 应用的后台进程、命令行工具、游戏等）不读系统代理，流量直接出去，无法被代理。

现有的解决方案：
- **Hiddify / Clash Verge 的 TUN 模式**：在 macOS 上存在 `auto_detect_interface` 失效的问题，代理服务器自身的流量被 TUN 捕获形成路由回环
- **sing-box 官方客户端（SFM）**：通过 Network Extension 解决，但中国区 App Store 不可用
- **proxychains**：hook 不稳定，部分应用漏掉

TunProxy 通过 `route_exclude_address` 在路由层面排除代理服务器的 IP，绕过了 `auto_detect_interface` 在 macOS 上的缺陷，实现了稳定的全局 TUN 代理。

## 功能

- 菜单栏常驻，盾牌图标指示连接状态
- 订阅链接导入（支持 VMess、Shadowsocks 旧格式）
- PAC 文件分流（通过 JS 引擎执行 `FindProxyForURL`）
- 节点切换、连通性检测
- 启动时自动连接上次使用的节点
- Privileged Helper Daemon（首次安装后不再弹密码）
- 开机启动

## 安装

```bash
# 编译
make build

# 安装到 /Applications
make install

# 确保 sing-box 二进制存在
cp /path/to/sing-box ~/.tun-proxy/sing-box
```

需要 sing-box 1.11.x（1.12+ 在 macOS 上有 `missing default interface` 问题）。

## 使用

1. 启动 TunProxy.app
2. 首次启动弹出系统授权框安装 helper（仅一次）
3. 点击「设置订阅链接」填入订阅 URL
4. 点击「更新订阅」拉取节点
5. 点击「连接」或选择节点

## 配置文件

所有配置存储在 `~/.tun-proxy/`：

```
~/.tun-proxy/
├── config.json      # 订阅链接、节点、PAC 路径
├── singbox.json     # 生成的 sing-box 配置
├── sing-box         # sing-box 二进制
├── pac.js           # PAC 文件副本
└── tun-proxy.log    # 日志
```

## 技术原理

1. 创建 TUN 虚拟网卡接管所有流量
2. 通过 `route_exclude_address` 排除代理服务器 IP，防止路由回环
3. 启动时自动解析代理服务器域名获取最新 IP
4. PAC 分流：用 goja（Go JS 引擎）执行 PAC 文件，对每个域名调用 `FindProxyForURL` 判断走代理还是直连
5. Privileged Helper 以 root 运行 sing-box（TUN 需要 root 权限创建网卡）

## 依赖

- [sing-box](https://github.com/SagerNet/sing-box) 1.11.x
- [systray](https://github.com/getlantern/systray) — 菜单栏
- [goja](https://github.com/dop251/goja) — JS 引擎（PAC 执行）

## License

MIT
