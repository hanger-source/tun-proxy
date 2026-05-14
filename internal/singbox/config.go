package singbox

import (
	"net"
	"tun-proxy/internal/config"
	"tun-proxy/internal/rules"
)

func ResolveServerIPs(nodes []config.Node) []string {
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

func buildDNSRules(proxyDomains []string, ruleSet *rules.Rules) []map[string]interface{} {
	var dnsRules []map[string]interface{}
	if len(proxyDomains) > 0 {
		dnsRules = append(dnsRules, map[string]interface{}{
			"domain": proxyDomains, "server": "dns-direct",
		})
	}
	if ruleSet != nil && len(ruleSet.DirectDomains) > 0 {
		dnsRules = append(dnsRules, map[string]interface{}{
			"domain_suffix": ruleSet.DirectDomains, "server": "dns-direct",
		})
	}
	return dnsRules
}

func GenerateConfig(nodes []config.Node, selected int, excludeIPs []string, ruleSet *rules.Rules) map[string]interface{} {
	var outboundNames []string
	var outbounds []map[string]interface{}

	for _, n := range nodes {
		tag := n.Name
		if tag == "" {
			tag = n.Server
		}
		outboundNames = append(outboundNames, tag)

		server := n.Server
		if net.ParseIP(server) == nil {
			if addrs, err := net.LookupHost(server); err == nil && len(addrs) > 0 {
				server = addrs[0]
			}
		}

		ob := map[string]interface{}{
			"tag":         tag,
			"type":        n.Type,
			"server":      server,
			"server_port": n.Port,
		}
		if n.Type == "vmess" {
			ob["uuid"] = n.UUID
			ob["security"] = "auto"
			ob["authenticated_length"] = true
			ob["packet_encoding"] = "xudp"
		} else if n.Type == "shadowsocks" {
			ob["method"] = n.Method
			ob["password"] = n.Password
		}
		outbounds = append(outbounds, ob)
	}

	defaultNode := ""
	if selected < len(outboundNames) {
		defaultNode = outboundNames[selected]
	}

	allOutbounds := []map[string]interface{}{
		{"type": "selector", "tag": "proxy", "outbounds": outboundNames, "default": defaultNode},
	}
	allOutbounds = append(allOutbounds, outbounds...)
	allOutbounds = append(allOutbounds, map[string]interface{}{"type": "direct", "tag": "direct"})

	excludeAddrs := append([]string{}, excludeIPs...)

	var proxyDomains []string
	for _, n := range nodes {
		if n.Server != "" {
			proxyDomains = append(proxyDomains, n.Server)
		}
	}

	routeRules := []map[string]interface{}{
		{"ip_is_private": true, "outbound": "direct"},
	}
	if ruleSet != nil {
		if len(ruleSet.ProxyDomains) > 0 {
			routeRules = append(routeRules, map[string]interface{}{"domain": ruleSet.ProxyDomains, "outbound": "proxy"})
		}
		if len(ruleSet.DirectDomains) > 0 {
			routeRules = append(routeRules, map[string]interface{}{"domain_suffix": ruleSet.DirectDomains, "outbound": "direct"})
		}
		if len(ruleSet.DirectCIDRs) > 0 {
			routeRules = append(routeRules, map[string]interface{}{"ip_cidr": ruleSet.DirectCIDRs, "outbound": "direct"})
		}
	}

	return map[string]interface{}{
		"log": map[string]interface{}{"level": "info", "timestamp": true},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{
				{"tag": "dns-remote", "address": "tcp://1.1.1.1", "detour": "proxy"},
				{"tag": "dns-direct", "address": "local", "detour": "direct"},
			},
			"rules": buildDNSRules(proxyDomains, ruleSet),
			"final":             "dns-remote",
			"strategy":          "prefer_ipv4",
			"independent_cache": true,
		},
		"inbounds": []map[string]interface{}{
			{
				"type":                       "tun",
				"tag":                        "tun-in",
				"address":                   []string{"172.19.0.1/28"},
				"auto_route":                true,
				"strict_route":              true,
				"stack":                     "gvisor",
				"sniff":                     true,
				"sniff_override_destination": false,
				"route_exclude_address":     excludeAddrs,
			},
		},
		"outbounds": allOutbounds,
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			"rules":                 routeRules,
			"final":                 "proxy",
		},
	}
}
