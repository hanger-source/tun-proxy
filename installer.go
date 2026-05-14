package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Security -framework Foundation
#include <Security/Security.h>
#include <stdlib.h>

int installHelper(const char* script) {
    AuthorizationRef auth;
    OSStatus status = AuthorizationCreate(NULL, kAuthorizationEmptyEnvironment,
        kAuthorizationFlagDefaults, &auth);
    if (status != errAuthorizationSuccess) return 1;

    AuthorizationItem items = {kAuthorizationRightExecute, 0, NULL, 0};
    AuthorizationRights rights = {1, &items};
    AuthorizationFlags flags = kAuthorizationFlagDefaults |
        kAuthorizationFlagInteractionAllowed |
        kAuthorizationFlagPreAuthorize |
        kAuthorizationFlagExtendRights;

    status = AuthorizationCopyRights(auth, &rights, NULL, flags, NULL);
    if (status != errAuthorizationSuccess) {
        AuthorizationFree(auth, kAuthorizationFlagDefaults);
        return 2;
    }

    // Execute install script with privileges
    char* args[] = {"-c", (char*)script, NULL};
    FILE* pipe = NULL;

    #pragma clang diagnostic push
    #pragma clang diagnostic ignored "-Wdeprecated-declarations"
    status = AuthorizationExecuteWithPrivileges(auth, "/bin/sh", kAuthorizationFlagDefaults, args, &pipe);
    #pragma clang diagnostic pop

    if (pipe) {
        char buf[256];
        while (fgets(buf, sizeof(buf), pipe)) {}
        fclose(pipe);
    }

    AuthorizationFree(auth, kAuthorizationFlagDefaults);
    return (status == errAuthorizationSuccess) ? 0 : 3;
}
*/
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	helperInstallPath = "/usr/local/bin/tun-proxy-helper"
	plistInstallPath  = "/Library/LaunchDaemons/com.hanger.tun-proxy.helper.plist"
	helperSockPath    = "/var/run/tun-proxy.sock"
)

func isHelperInstalled() bool {
	_, err := os.Stat(helperInstallPath)
	if err != nil {
		return false
	}
	// Check if daemon is running in system domain
	out, _ := exec.Command("launchctl", "print", "system/com.hanger.tun-proxy.helper").CombinedOutput()
	return !strings.Contains(string(out), "Could not find")
}

func installHelperIfNeeded() error {
	if isHelperInstalled() {
		logInfo("helper already installed")
		return nil
	}

	logInfo("installing privileged helper...")

	// Get paths
	appDir := getAppResourcesDir()
	helperSrc := filepath.Join(appDir, "tun-proxy-helper")
	plistSrc := filepath.Join(appDir, "com.hanger.tun-proxy.helper.plist")

	// Fallback: look next to the binary
	if _, err := os.Stat(helperSrc); err != nil {
		exePath, _ := os.Executable()
		exeDir := filepath.Dir(exePath)
		helperSrc = filepath.Join(exeDir, "..", "Resources", "tun-proxy-helper")
		plistSrc = filepath.Join(exeDir, "..", "Resources", "com.hanger.tun-proxy.helper.plist")
	}
	if _, err := os.Stat(helperSrc); err != nil {
		// Try helper dir in project
		home, _ := os.UserHomeDir()
		helperSrc = filepath.Join(home, "projects", "tun-proxy", "helper", "tun-proxy-helper")
		plistSrc = filepath.Join(home, "projects", "tun-proxy", "helper", "com.hanger.tun-proxy.helper.plist")
	}

	if _, err := os.Stat(helperSrc); err != nil {
		return fmt.Errorf("helper binary not found")
	}

	// Install script
	script := fmt.Sprintf(
		`cp "%s" "%s" && chmod 755 "%s" && chown root:wheel "%s" && `+
			`cp "%s" "%s" && chmod 644 "%s" && chown root:wheel "%s" && `+
			`launchctl bootout system/%s 2>/dev/null; `+
			`launchctl bootstrap system "%s"`,
		helperSrc, helperInstallPath, helperInstallPath, helperInstallPath,
		plistSrc, plistInstallPath, plistInstallPath, plistInstallPath,
		"com.hanger.tun-proxy.helper",
		plistInstallPath,
	)

	result := C.installHelper(C.CString(script))
	if result != 0 {
		return fmt.Errorf("authorization failed (code %d)", result)
	}

	logInfo("helper installed successfully")
	return nil
}

func getAppResourcesDir() string {
	exePath, _ := os.Executable()
	return filepath.Join(filepath.Dir(exePath), "..", "Resources")
}
