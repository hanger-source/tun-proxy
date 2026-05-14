package pac

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Rules struct {
	ProxyDomains  []string
	DirectDomains []string
	DirectCIDRs   []string
}

type ruleSet struct {
	Version int        `json:"version"`
	Rules   []ruleItem `json:"rules"`
}

type ruleItem struct {
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	Domain       []string `json:"domain,omitempty"`
	IPCIDR       []string `json:"ip_cidr,omitempty"`
}

var cache *Rules
var cacheHash string

// GetRules loads rules from a directory containing ruleset-proxy.json and ruleset-direct.json.
// If path points to a file, uses its parent directory.
// Returns cached result if files haven't changed.
func GetRules(path string) *Rules {
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	hash := dirHash(dir)
	if hash == cacheHash && cache != nil {
		return cache
	}

	cache = loadRules(dir)
	cacheHash = hash
	return cache
}

func ClearCache() {
	cache = nil
	cacheHash = ""
}

func loadRules(dir string) *Rules {
	rules := &Rules{}

	// Load proxy ruleset
	if data, err := os.ReadFile(filepath.Join(dir, "ruleset-proxy.json")); err == nil {
		var rs ruleSet
		if json.Unmarshal(data, &rs) == nil {
			for _, r := range rs.Rules {
				rules.ProxyDomains = append(rules.ProxyDomains, r.DomainSuffix...)
				rules.ProxyDomains = append(rules.ProxyDomains, r.Domain...)
			}
		}
	}

	// Load direct ruleset
	if data, err := os.ReadFile(filepath.Join(dir, "ruleset-direct.json")); err == nil {
		var rs ruleSet
		if json.Unmarshal(data, &rs) == nil {
			for _, r := range rs.Rules {
				for _, d := range r.DomainSuffix {
					if !strings.HasPrefix(d, ".") {
						d = "." + d
					}
					rules.DirectDomains = append(rules.DirectDomains, d)
				}
				rules.DirectDomains = append(rules.DirectDomains, r.Domain...)
				rules.DirectCIDRs = append(rules.DirectCIDRs, r.IPCIDR...)
			}
		}
	}

	return rules
}

func dirHash(dir string) string {
	h := md5.New()
	for _, name := range []string{"ruleset-proxy.json", "ruleset-direct.json"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
