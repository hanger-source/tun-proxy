package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

type PACRules struct {
	ProxyDomains  []string
	DirectDomains []string
	DirectCIDRs   []string
}

var pacCache *PACRules
var pacFileHash string

func GetPACRules(path string) *PACRules {
	if path == "" {
		return nil
	}
	hash := fileHash(path)
	if hash == pacFileHash && pacCache != nil {
		return pacCache
	}
	pacCache = parsePACFile(path)
	pacFileHash = hash
	return pacCache
}

func ClearPACCache() {
	pacCache = nil
	pacFileHash = ""
}

func parsePACFile(path string) *PACRules {
	data, err := os.ReadFile(path)
	if err != nil {
		logWarn("cannot read PAC file: %v", err)
		return nil
	}
	content := string(data)

	// Extract only the declared domain arrays (not all strings in the file)
	declaredDomains := extractJSArray(content, "var domains")
	declaredHosts := extractJSArray(content, "var hostArr")

	// Build test list: only declared domains (typically < 200)
	var testDomains []string
	for _, d := range declaredDomains {
		if isValidDomain(d) {
			testDomains = append(testDomains, d)
		}
	}
	for _, d := range declaredHosts {
		clean := strings.TrimPrefix(d, "*.")
		if isValidDomain(clean) {
			testDomains = append(testDomains, clean)
		}
	}

	logInfo("PAC: testing %d declared domains via FindProxyForURL", len(testDomains))

	// Execute PAC with goja
	vm := goja.New()
	vm.Set("isPlainHostName", func(host string) bool { return !strings.Contains(host, ".") })
	vm.Set("dnsDomainIs", func(host, domain string) bool {
		return host == domain || strings.HasSuffix(host, "."+domain)
	})
	vm.Set("shExpMatch", func(str, pattern string) bool {
		re := "^" + regexp.QuoteMeta(pattern) + "$"
		re = strings.ReplaceAll(re, `\*`, `.*`)
		re = strings.ReplaceAll(re, `\?`, `.`)
		matched, _ := regexp.MatchString(re, str)
		return matched
	})
	vm.Set("isResolvable", func(host string) bool { return false })
	vm.Set("dnsResolve", func(host string) string { return "0.0.0.0" })
	vm.Set("isInNet", func(ip, subnet, mask string) bool { return false })

	_, err = vm.RunString(content)
	if err != nil {
		logError("PAC JS execution failed: %v", err)
		return nil
	}

	findProxy, ok := goja.AssertFunction(vm.Get("FindProxyForURL"))
	if !ok {
		logError("FindProxyForURL not found in PAC")
		return nil
	}

	rules := &PACRules{}
	for _, domain := range testDomains {
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

	// Extract CIDR ranges
	rules.DirectCIDRs = extractCIDRPairs(content)

	logInfo("PAC result: %d proxy domains, %d direct domains, %d direct CIDRs",
		len(rules.ProxyDomains), len(rules.DirectDomains), len(rules.DirectCIDRs))
	return rules
}

func extractJSArray(content, varDecl string) []string {
	idx := strings.Index(content, varDecl)
	if idx < 0 {
		return nil
	}
	start := strings.Index(content[idx:], "[")
	if start < 0 {
		return nil
	}
	start += idx
	end := strings.Index(content[start:], "];")
	if end < 0 {
		return nil
	}
	block := content[start : start+end+1]
	re := regexp.MustCompile(`"([^"]+)"`)
	matches := re.FindAllStringSubmatch(block, -1)
	var result []string
	for _, m := range matches {
		result = append(result, m[1])
	}
	return result
}

func extractCIDRPairs(content string) []string {
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

func isValidDomain(s string) bool {
	if strings.Contains(s, "*") || strings.Contains(s, " ") {
		return false
	}
	if regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`).MatchString(s) {
		return false
	}
	return strings.Contains(s, ".")
}

func fileHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}
