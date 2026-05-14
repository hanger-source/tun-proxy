package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type Node struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // vmess, shadowsocks
	Server   string `json:"server"`
	Port     int    `json:"port"`
	UUID     string `json:"uuid,omitempty"`
	Security string `json:"security,omitempty"`
	Method   string `json:"method,omitempty"`
	Password string `json:"password,omitempty"`
}

func ParseSubscription(url string) ([]Node, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取订阅失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Base64 decode the subscription content
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
	if err != nil {
		// Try RawStdEncoding
		decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(body)))
		if err != nil {
			return nil, fmt.Errorf("base64 解码失败: %v", err)
		}
	}

	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	var nodes []Node
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "vmess://") {
			n, err := parseVmess(line)
			if err == nil {
				nodes = append(nodes, n)
			}
		} else if strings.HasPrefix(line, "ss://") {
			n, err := parseSS(line)
			if err == nil {
				nodes = append(nodes, n)
			}
		}
	}
	return nodes, nil
}

func parseVmess(link string) (Node, error) {
	raw := strings.TrimPrefix(link, "vmess://")
	// Pad base64 if needed
	if m := len(raw) % 4; m != 0 {
		raw += strings.Repeat("=", 4-m)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, "vmess://"))
		if err != nil {
			return Node{}, err
		}
	}

	var v struct {
		Ps   string      `json:"ps"`
		Add  string      `json:"add"`
		Port interface{} `json:"port"`
		ID   string      `json:"id"`
		Type string      `json:"type"`
	}
	if err := json.Unmarshal(decoded, &v); err != nil {
		return Node{}, err
	}

	port := 0
	switch p := v.Port.(type) {
	case float64:
		port = int(p)
	case string:
		port, _ = strconv.Atoi(p)
	}

	return Node{
		Name:     v.Ps,
		Type:     "vmess",
		Server:   v.Add,
		Port:     port,
		UUID:     v.ID,
		Security: "auto",
	}, nil
}

func parseSS(link string) (Node, error) {
	// Format: ss://base64(method:password@host:port)#name
	// or SIP002: ss://base64(method:password)@host:port#name
	raw := strings.TrimPrefix(link, "ss://")

	name := ""
	if idx := strings.LastIndex(raw, "#"); idx >= 0 {
		name = raw[idx+1:]
		raw = raw[:idx]
	}

	// Check if there's a @ in plaintext (SIP002 format)
	if idx := strings.Index(raw, "@"); idx > 0 && !isBase64Only(raw[:idx]) {
		// Not handling this case for now
	}

	// Try legacy format: entire thing is base64
	userinfo := raw
	if idx := strings.Index(raw, "@"); idx > 0 {
		// SIP002: base64part@host:port
		userinfo = raw[:idx]
	}

	// Decode base64 part
	decoded, err := decodeB64(userinfo)
	if err != nil {
		return Node{}, err
	}

	// Legacy format: method:password@host:port
	if atIdx := strings.LastIndex(decoded, "@"); atIdx > 0 {
		credentials := decoded[:atIdx]
		hostPort := decoded[atIdx+1:]

		colonIdx := strings.Index(credentials, ":")
		if colonIdx <= 0 {
			return Node{}, fmt.Errorf("invalid credentials")
		}
		method := credentials[:colonIdx]
		password := credentials[colonIdx+1:]

		lastColon := strings.LastIndex(hostPort, ":")
		if lastColon <= 0 {
			return Node{}, fmt.Errorf("invalid host:port")
		}
		host := hostPort[:lastColon]
		port, _ := strconv.Atoi(hostPort[lastColon+1:])

		return Node{
			Name:     name,
			Type:     "shadowsocks",
			Server:   host,
			Port:     port,
			Method:   method,
			Password: password,
		}, nil
	}

	// SIP002 format: decoded is method:password, host:port after @
	parts := strings.SplitN(decoded, ":", 2)
	if len(parts) != 2 {
		return Node{}, fmt.Errorf("invalid ss format")
	}
	method := parts[0]
	password := parts[1]

	// Get host:port from after @ in original
	if idx := strings.Index(raw, "@"); idx > 0 {
		hostPort := raw[idx+1:]
		lastColon := strings.LastIndex(hostPort, ":")
		if lastColon > 0 {
			host := hostPort[:lastColon]
			port, _ := strconv.Atoi(hostPort[lastColon+1:])
			return Node{
				Name:     name,
				Type:     "shadowsocks",
				Server:   host,
				Port:     port,
				Method:   method,
				Password: password,
			}, nil
		}
	}

	return Node{}, fmt.Errorf("cannot parse ss link")
}

func decodeB64(s string) (string, error) {
	// Try standard base64
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return string(decoded), nil
	}
	// Try URL-safe
	if decoded, err := base64.URLEncoding.DecodeString(s); err == nil {
		return string(decoded), nil
	}
	// Try raw
	if decoded, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return string(decoded), nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return string(decoded), nil
	}
	return "", fmt.Errorf("base64 decode failed")
}

func isBase64Only(s string) bool {
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=') {
			return false
		}
	}
	return true
}
