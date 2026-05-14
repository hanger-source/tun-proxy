package main

func GenerateSingBoxConfig(nodes []Node, selected int, excludeIPs []string, pacRules *PACRules) map[string]interface{} {
	// Build outbounds
	var outboundNames []string
	var outbounds []map[string]interface{}

	for _, n := range nodes {
		tag := n.Name
		if tag == "" {
			tag = n.Server
		}
		outboundNames = append(outboundNames, tag)

		ob := map[string]interface{}{
			"tag":         tag,
			"type":        n.Type,
			"server":      n.Server,
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

	// Selector
	defaultNode := ""
	if selected < len(outboundNames) {
		defaultNode = outboundNames[selected]
	}
	selector := map[string]interface{}{
		"type":      "selector",
		"tag":       "proxy",
		"outbounds": outboundNames,
		"default":   defaultNode,
	}

	// Direct
	direct := map[string]interface{}{
		"type": "direct",
		"tag":  "direct",
	}

	allOutbounds := []map[string]interface{}{selector}
	allOutbounds = append(allOutbounds, outbounds...)
	allOutbounds = append(allOutbounds, direct)

	// route_exclude_address
	excludeAddrs := []string{"223.5.5.5/32", "10.0.0.0/8"}
	excludeAddrs = append(excludeAddrs, excludeIPs...)

	// Collect proxy server domains for DNS direct resolution
	var proxyDomains []string
	for _, n := range nodes {
		if n.Server != "" {
			proxyDomains = append(proxyDomains, n.Server)
		}
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{"level": "warn", "timestamp": true},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{
				{"tag": "dns-remote", "address": "tcp://8.8.8.8", "detour": "proxy"},
				{"tag": "dns-direct", "address": "223.5.5.5", "detour": "direct"},
			},
			"rules": []map[string]interface{}{
				{"domain_suffix": []string{".cn"}, "server": "dns-direct"},
				{"domain": proxyDomains, "server": "dns-direct"},
			},
			"final":             "dns-remote",
			"strategy":          "prefer_ipv4",
			"independent_cache": true,
		},
		"inbounds": []map[string]interface{}{
			{
				"type":                  "tun",
				"tag":                   "tun-in",
				"address":              []string{"172.19.0.1/28"},
				"auto_route":           true,
				"strict_route":         true,
				"stack":                "gvisor",
				"route_exclude_address": excludeAddrs,
			},
		},
		"outbounds": allOutbounds,
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			"rules": func() []map[string]interface{} {
				rules := []map[string]interface{}{
					{"ip_is_private": true, "outbound": "direct"},
					{"domain_suffix": ".cn", "outbound": "direct"},
				}
				if pacRules != nil {
					if len(pacRules.ProxyDomains) > 0 {
						rules = append(rules, map[string]interface{}{
							"domain": pacRules.ProxyDomains,
							"outbound": "proxy",
						})
					}
					if len(pacRules.DirectDomains) > 0 {
						rules = append(rules, map[string]interface{}{
							"domain_suffix": pacRules.DirectDomains,
							"outbound":      "direct",
						})
					}
					if len(pacRules.DirectCIDRs) > 0 {
						rules = append(rules, map[string]interface{}{
							"ip_cidr":  pacRules.DirectCIDRs,
							"outbound": "direct",
						})
					}
				}
				return rules
			}(),
			"final": "proxy",
		},
	}

	return config
}
