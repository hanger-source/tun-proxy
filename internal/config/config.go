package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Node struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	UUID     string `json:"uuid,omitempty"`
	Security string `json:"security,omitempty"`
	Method   string `json:"method,omitempty"`
	Password string `json:"password,omitempty"`
}

type Config struct {
	SubscribeURL string `json:"subscribe_url"`
	PACPath      string `json:"pac_path"`
	Nodes        []Node `json:"nodes"`
	SelectedNode int    `json:"selected_node"`
}

func Dir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tun-proxy")
	os.MkdirAll(dir, 0755)
	return dir
}

func Path() string {
	return filepath.Join(Dir(), "config.json")
}

func Load() (*Config, error) {
	c := &Config{}
	data, err := os.ReadFile(Path())
	if err != nil {
		return c, nil // fresh config
	}
	if err := json.Unmarshal(data, c); err != nil {
		return c, err
	}
	return c, nil
}

func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0644)
}
