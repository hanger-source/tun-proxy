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

- 菜单栏常驻，盾牌图标指示连接状态（自动适配深色/浅色模式）
- 订阅链接导入（支持 VMess、Shadowsocks 旧格式/SIP002）
- sing-box 原生规则集分流（`ruleset-proxy.json` / `ruleset-direct.json`）
- 节点切换、连通性检测、故障自动切换
- 启动时自动连接上次使用的节点
- 首次启动自动下载 sing-box
- Privileged Helper Daemon（首次安装后不再弹密码）
- 开机启动
- 域名路由测试（查看某个域名走代理还是直连）
- 路由日志查看

## 安装

```bash
# 编译
make build

# 安装到 /Applications
make install
```

首次启动会自动下载 sing-box 1.11.15 并安装 privileged helper。

## 使用

1. 启动 TunProxy.app
2. 首次启动弹出系统授权框安装 helper（仅一次）
3. 点击「设置订阅链接」填入订阅 URL
4. 点击「更新订阅」拉取节点
5. 自动连接，菜单栏图标变为实心盾牌

## 规则分流

TunProxy 使用 sing-box 原生规则集格式进行分流。规则文件放在 `~/.tun-proxy/` 目录下：

- `ruleset-proxy.json` — 匹配的域名强制走代理
- `ruleset-direct.json` — 匹配的域名/IP 强制直连
- 不匹配任何规则的流量 → 走代理（默认）
- 私有 IP → 直连

规则文件格式（sing-box rule-set）：
```json
{
  "version": 1,
  "rules": [
    {
      "domain_suffix": ["example.com", "example.org"],
      "ip_cidr": ["10.0.0.0/8"]
    }
  ]
}
```

## 配置文件

所有配置存储在 `~/.tun-proxy/`：

```
~/.tun-proxy/
├── config.json           # 订阅链接、节点列表、选中节点
├── singbox.json          # 生成的 sing-box 配置（自动生成）
├── sing-box              # sing-box 二进制（自动下载）
├── ruleset-proxy.json    # 代理规则集
├── ruleset-direct.json   # 直连规则集
└── tun-proxy.log         # 日志
```

## 技术原理

1. 创建 TUN 虚拟网卡（utun）接管所有流量
2. TLS SNI 嗅探（sniff）提取域名，实现基于域名的路由分流
3. 通过 `route_exclude_address` 排除代理服务器 IP，防止路由回环
4. 启动时自动解析代理服务器域名获取最新 IP
5. 规则分流：加载 JSON 规则集，生成 sing-box 路由和 DNS 配置
6. 直连域名使用系统 DNS 解析，避免内网域名经代理 DNS 解析到错误 IP
7. Privileged Helper 以 root 运行 sing-box（TUN 需要 root 权限创建网卡）
8. App 通过 Unix Socket 与 Helper 通信（启动/停止 sing-box）

## 项目结构

```
├── main.go                    # UI（systray 菜单栏）
├── app.go                     # 应用逻辑封装
├── internal/
│   ├── config/                # 配置管理
│   ├── engine/                # sing-box 生命周期 + helper 通信
│   ├── rules/                 # 规则集加载
│   ├── singbox/               # sing-box 配置生成
│   └── subscription/          # 订阅解析
├── helper/                    # Privileged Helper Daemon
├── installer.go               # Helper 安装（AuthorizationCreate）
├── download.go                # sing-box 自动下载
└── assets/                    # 菜单栏图标
```

## 依赖

- [sing-box](https://github.com/SagerNet/sing-box) 1.11.x（自动下载）
- [systray](https://github.com/getlantern/systray) — 菜单栏
- macOS Security.framework — 权限提升

## License

MIT
