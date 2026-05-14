package pac

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

type Rules struct {
	ProxyDomains  []string
	DirectDomains []string
	DirectCIDRs   []string
}

var cache *Rules
var cacheHash string

func GetRules(path string) *Rules {
	if path == "" {
		return nil
	}
	hash := fileHash(path)
	if hash == cacheHash && cache != nil {
		return cache
	}
	cache = parse(path)
	cacheHash = hash
	return cache
}

func ClearCache() {
	cache = nil
	cacheHash = ""
}

func parse(path string) *Rules {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

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

	if _, err := vm.RunString(content); err != nil {
		return nil
	}

	var testDomains []string
	if arr := vm.Get("domains"); arr != nil {
		if list, ok := arr.Export().([]interface{}); ok {
			for _, v := range list {
				if s, ok := v.(string); ok && isValidDomain(s) {
					testDomains = append(testDomains, s)
				}
			}
		}
	}
	if arr := vm.Get("hostArr"); arr != nil {
		if list, ok := arr.Export().([]interface{}); ok {
			for _, v := range list {
				if s, ok := v.(string); ok {
					clean := strings.TrimPrefix(s, "*.")
					if isValidDomain(clean) {
						testDomains = append(testDomains, clean)
					}
				}
			}
		}
	}

	findProxy, ok := goja.AssertFunction(vm.Get("FindProxyForURL"))
	if !ok {
		return nil
	}

	rules := &Rules{}
	for _, domain := range testDomains {
		r, err := findProxy(goja.Undefined(), vm.ToValue("https://"+domain+"/"), vm.ToValue(domain))
		if err != nil {
			continue
		}
		resultStr := r.String()
		if resultStr == "DIRECT" || strings.HasPrefix(resultStr, "DIRECT") {
			rules.DirectDomains = append(rules.DirectDomains, "."+domain)
		} else {
			rules.ProxyDomains = append(rules.ProxyDomains, domain)
		}
	}

	rules.DirectCIDRs = extractCIDRPairs(content)
	return rules
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
