package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

type PACRules struct {
	ProxyDomains  []string // 走代理
	DirectDomains []string // 直连
	DirectCIDRs   []string // 直连 IP 段
}

// ParsePACFile executes PAC JS with goja, extracts all declared domains,
// runs each through FindProxyForURL to classify as proxy or direct.
func ParsePACFile(path string) *PACRules {
	data, err := os.ReadFile(path)
	if err != nil {
		logWarn("cannot read PAC file: %v", err)
		return nil
	}
	content := string(data)

	// Create JS runtime
	vm := goja.New()

	// Provide PAC helper functions
	vm.Set("isPlainHostName", func(host string) bool {
		return !strings.Contains(host, ".")
	})
	vm.Set("dnsDomainIs", func(host, domain string) bool {
		return host == domain || strings.HasSuffix(host, "."+domain)
	})
	vm.Set("shExpMatch", func(str, pattern string) bool {
		// Convert shell pattern to regex
		re := "^" + regexp.QuoteMeta(pattern) + "$"
		re = strings.ReplaceAll(re, `\*`, `.*`)
		re = strings.ReplaceAll(re, `\?`, `.`)
		matched, _ := regexp.MatchString(re, str)
		return matched
	})
	vm.Set("isResolvable", func(host string) bool {
		return false // Skip DNS resolution during parsing
	})
	vm.Set("dnsResolve", func(host string) string {
		return "0.0.0.0"
	})
	vm.Set("isInNet", func(ip, subnet, mask string) bool {
		return false
	})

	// Execute PAC script
	_, err = vm.RunString(content)
	if err != nil {
		logError("PAC JS execution failed: %v", err)
		return nil
	}

	// Extract all domains declared in the PAC file
	allDomains := extractAllDomains(content)
	logInfo("extracted %d domains from PAC file to test", len(allDomains))

	// Run each domain through FindProxyForURL
	findProxy, ok := goja.AssertFunction(vm.Get("FindProxyForURL"))
	if !ok {
		logError("FindProxyForURL not found in PAC")
		return nil
	}

	rules := &PACRules{}
	for _, domain := range allDomains {
		url := "https://" + domain + "/"
		result, err := findProxy(goja.Undefined(), vm.ToValue(url), vm.ToValue(domain))
		if err != nil {
			continue
		}
		resultStr := result.String()
		if strings.Contains(resultStr, "DIRECT") {
			rules.DirectDomains = append(rules.DirectDomains, "."+domain)
		} else {
			rules.ProxyDomains = append(rules.ProxyDomains, domain)
		}
	}

	// Also extract CIDR ranges directly (since we skip DNS in PAC execution)
	rules.DirectCIDRs = extractCIDRsFromJS(content)

	logInfo("PAC result: %d proxy domains, %d direct domains, %d direct CIDRs",
		len(rules.ProxyDomains), len(rules.DirectDomains), len(rules.DirectCIDRs))

	return rules
}

// extractAllDomains pulls all quoted domain-like strings from the PAC file
func extractAllDomains(content string) []string {
	re := regexp.MustCompile(`"(\*\.)?([a-zA-Z0-9][-a-zA-Z0-9]*(?:\.[a-zA-Z0-9][-a-zA-Z0-9]*)+)"`)
	matches := re.FindAllStringSubmatch(content, -1)

	seen := map[string]bool{}
	var domains []string
	for _, m := range matches {
		domain := m[2]
		if strings.Contains(domain, "*") {
			continue
		}
		// Skip IP-like patterns
		if regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`).MatchString(domain) {
			continue
		}
		if !seen[domain] {
			seen[domain] = true
			domains = append(domains, domain)
		}
	}
	return domains
}

// extractCIDRsFromJS extracts CIDR entries from cidrArr in the PAC
func extractCIDRsFromJS(content string) []string {
	re := regexp.MustCompile(`\["(\d+\.\d+\.\d+\.\d+)",\s*"(\d+\.\d+\.\d+\.\d+)"\]`)
	matches := re.FindAllStringSubmatch(content, -1)

	var cidrs []string
	for _, m := range matches {
		bits := maskBits(m[2])
		if bits > 0 {
			cidrs = append(cidrs, fmt.Sprintf("%s/%d", m[1], bits))
		}
	}
	return cidrs
}

func maskBits(mask string) int {
	parts := strings.Split(mask, ".")
	if len(parts) != 4 {
		return 0
	}
	bits := 0
	for _, p := range parts {
		var n int
		for _, c := range p {
			n = n*10 + int(c-'0')
		}
		for i := 7; i >= 0; i-- {
			if n&(1<<i) != 0 {
				bits++
			} else {
				return bits
			}
		}
	}
	return bits
}
