package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"tun-proxy/internal/config"
)

func Fetch(url string) ([]config.Node, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	decoded, err := decodeB64(strings.TrimSpace(string(body)))
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(decoded), "\n")
	var nodes []config.Node
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "vmess://") {
			if n, err := parseVmess(line); err == nil {
				nodes = append(nodes, n)
			}
		} else if strings.HasPrefix(line, "ss://") {
			if n, err := parseSS(line); err == nil {
				nodes = append(nodes, n)
			}
		}
	}
	return nodes, nil
}

func parseVmess(link string) (config.Node, error) {
	raw := strings.TrimPrefix(link, "vmess://")
	decoded, err := decodeB64(raw)
	if err != nil {
		return config.Node{}, err
	}

	var v struct {
		Ps   string      `json:"ps"`
		Add  string      `json:"add"`
		Port interface{} `json:"port"`
		ID   string      `json:"id"`
	}
	if err := json.Unmarshal([]byte(decoded), &v); err != nil {
		return config.Node{}, err
	}

	port := 0
	switch p := v.Port.(type) {
	case float64:
		port = int(p)
	case string:
		port, _ = strconv.Atoi(p)
	}

	return config.Node{
		Name:     v.Ps,
		Type:     "vmess",
		Server:   v.Add,
		Port:     port,
		UUID:     v.ID,
		Security: "auto",
	}, nil
}

func parseSS(link string) (config.Node, error) {
	raw := strings.TrimPrefix(link, "ss://")
	name := ""
	if idx := strings.LastIndex(raw, "#"); idx >= 0 {
		name = raw[idx+1:]
		raw = raw[:idx]
	}

	userinfo := raw
	if idx := strings.Index(raw, "@"); idx > 0 {
		userinfo = raw[:idx]
	}

	decoded, err := decodeB64(userinfo)
	if err != nil {
		return config.Node{}, err
	}

	// Legacy format: method:password@host:port
	if atIdx := strings.LastIndex(decoded, "@"); atIdx > 0 {
		credentials := decoded[:atIdx]
		hostPort := decoded[atIdx+1:]
		colonIdx := strings.Index(credentials, ":")
		if colonIdx <= 0 {
			return config.Node{}, fmt.Errorf("invalid credentials")
		}
		lastColon := strings.LastIndex(hostPort, ":")
		if lastColon <= 0 {
			return config.Node{}, fmt.Errorf("invalid host:port")
		}
		port, _ := strconv.Atoi(hostPort[lastColon+1:])
		return config.Node{
			Name:     name,
			Type:     "shadowsocks",
			Server:   hostPort[:lastColon],
			Port:     port,
			Method:   credentials[:colonIdx],
			Password: credentials[colonIdx+1:],
		}, nil
	}

	// SIP002 format
	parts := strings.SplitN(decoded, ":", 2)
	if len(parts) != 2 {
		return config.Node{}, fmt.Errorf("invalid ss format")
	}
	if idx := strings.Index(raw, "@"); idx > 0 {
		hostPort := raw[idx+1:]
		lastColon := strings.LastIndex(hostPort, ":")
		if lastColon > 0 {
			port, _ := strconv.Atoi(hostPort[lastColon+1:])
			return config.Node{
				Name:     name,
				Type:     "shadowsocks",
				Server:   hostPort[:lastColon],
				Port:     port,
				Method:   parts[0],
				Password: parts[1],
			}, nil
		}
	}
	return config.Node{}, fmt.Errorf("cannot parse ss link")
}

func decodeB64(s string) (string, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	} {
		if decoded, err := enc.DecodeString(s); err == nil {
			return string(decoded), nil
		}
	}
	return "", fmt.Errorf("base64 decode failed")
}
