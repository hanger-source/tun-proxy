package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

var logger *log.Logger

func initLogger() {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tun-proxy")
	os.MkdirAll(dir, 0755)
	logPath := filepath.Join(dir, "tun-proxy.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
		return
	}
	logger = log.New(f, "", log.LstdFlags)
	// Also print to stderr
	logger.SetOutput(os.Stderr)
	logger = log.New(f, "", 0)

	// Use custom format
	log.SetOutput(f)
	log.SetFlags(0)
}

func logInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	entry := fmt.Sprintf("%s [INFO]  %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	fmt.Fprintln(os.Stderr, entry)
	if logger != nil {
		logger.Println(entry)
	}
}

func logError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	entry := fmt.Sprintf("%s [ERROR] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	fmt.Fprintln(os.Stderr, entry)
	if logger != nil {
		logger.Println(entry)
	}
}

func logWarn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	entry := fmt.Sprintf("%s [WARN]  %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	fmt.Fprintln(os.Stderr, entry)
	if logger != nil {
		logger.Println(entry)
	}
}
