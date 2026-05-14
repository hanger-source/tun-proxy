package main

import (
	"os/exec"
	"strings"
	"time"
)

func promptInput(title, defaultValue string) string {
	script := `display dialog "` + title + `" default answer "` + defaultValue + `" buttons {"取消", "确定"} default button "确定" with title "TUN Proxy" giving up after 120`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return ""
	}
	result := string(out)
	if idx := strings.Index(result, "text returned:"); idx >= 0 {
		return strings.TrimSpace(result[idx+len("text returned:"):])
	}
	return ""
}

func showAlert(msg string) {
	script := `display notification "` + msg + `" with title "TUN Proxy"`
	exec.Command("osascript", "-e", script).Run()
}

func sleepMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
