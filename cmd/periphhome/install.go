// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"runtime"
)

func install(config string) error {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return setupSystemd(config)
	}
	return fmt.Errorf("please send a PR to implement me on %s", runtime.GOOS)
}

const systemdConfig = `
# See https://periph.io/x/home
[Unit]
Description=Runs periphhome automatically upon boot
Wants=network-online.target
After=network-online.target

[Service]
User={{.User}}
Group={{.Group}}
KillMode=mixed
Restart=always
TimeoutStopSec=20s
ExecStart={{.Command}}
Environment=GOTRACEBACK=all

# Allow binding to port 80:
# Systemd 229:
AmbientCapabilities=CAP_NET_BIND_SERVICE
# Systemd 228 and below:
#SecureBits=keep-caps
#Capabilities=cap_net_bind_service+pie
# Older systemd:
#PermissionsStartOnly=true
#ExecStartPre=/sbin/setcap 'cap_net_bind_service=+ep' {{.Executable}}

[Install]
WantedBy=default.target
`

// setupSystemd installs itself as a service via systemd.
func setupSystemd(config string) error {
	t, err := template.New("").Parse(systemdConfig)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// Generate the service file.
	buf := bytes.Buffer{}
	data := map[string]string{
		"User":       "pi",
		"Group":      "pi",
		"Command":    exe + " " + config + " run",
		"Executable": exe,
	}
	if err = t.Execute(&buf, data); err != nil {
		return err
	}

	cmd := exec.Command("sudo", "tee", "/etc/systemd/system/periphhome.service")
	cmd.Stdin = &buf
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("sudo", "systemctl", "daemon-reload")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("sudo", "systemctl", "enable", "periphhome.service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("Run \"sudo systemctl start periphhome.service\" to start the node or reboot.\n")
	return nil
}

/*
func systemdEscape(s string) (string, error) {
	buf := bytes.Buffer{}
	cmd := exec.Command("systemd-escape", s)
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}
*/
