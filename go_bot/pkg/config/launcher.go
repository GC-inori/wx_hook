package config

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var (
	displayPattern = regexp.MustCompile(`^:([0-9]{1,5})(?:\.([0-9]{1,2}))?$`)
	screenPattern  = regexp.MustCompile(`^([0-9]{1,5})x([0-9]{1,5})x(8|16|24|32)$`)
)

// DisplayNumber restricts managed displays to local X11 sockets; remote X servers
// require external lifecycle management (launcher.auto_start: false).
func (c LauncherConfig) DisplayNumber() (int, error) {
	parts := displayPattern.FindStringSubmatch(c.Display)
	if parts == nil {
		return 0, fmt.Errorf("launcher.display must be a local X11 display, for example :99 or :99.0")
	}
	n, _ := strconv.Atoi(parts[1])
	if n > 65535 {
		return 0, fmt.Errorf("launcher.display number exceeds 65535")
	}
	return n, nil
}

func (c LauncherConfig) Validate() error {
	if c.BridgeWaitTimeout < 5 {
		return fmt.Errorf("launcher.bridge_wait_timeout must be at least 5 seconds")
	}
	if c.XvfbWaitTimeout < 1 {
		return fmt.Errorf("launcher.xvfb_wait_timeout must be at least 1 second")
	}
	if c.QRPort < 0 || c.QRPort > 65535 {
		return fmt.Errorf("launcher.qr_port must be between 0 and 65535")
	}
	if c.LoginTimeout < 1 || c.LoginTimeout > 86400 {
		return fmt.Errorf("launcher.login_timeout must be between 1 and 86400 seconds")
	}
	if !c.AutoStart {
		return nil
	}
	if strings.TrimSpace(c.BridgeLib) == "" || strings.ContainsAny(c.BridgeLib, " \t\r\n:") {
		return fmt.Errorf("launcher.bridge_lib must be nonempty and cannot contain LD_PRELOAD separators (whitespace or colon)")
	}
	if _, err := c.DisplayNumber(); err != nil {
		return err
	}
	if c.UseXvfb {
		parts := screenPattern.FindStringSubmatch(c.XvfbScreen)
		if parts == nil {
			return fmt.Errorf("launcher.xvfb_screen must have the form WIDTHxHEIGHTxDEPTH")
		}
		w, _ := strconv.Atoi(parts[1])
		h, _ := strconv.Atoi(parts[2])
		if w < 320 || h < 240 || w > 8192 || h > 8192 {
			return fmt.Errorf("launcher.xvfb_screen dimensions must be between 320x240 and 8192x8192")
		}
	}
	if c.QRPort != 0 && c.QRBind != "localhost" && net.ParseIP(c.QRBind) == nil {
		return fmt.Errorf("launcher.qr_bind must be an IP address or localhost")
	}
	return nil
}
