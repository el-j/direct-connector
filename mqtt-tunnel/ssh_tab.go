package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// buildSSHTab constructs the SSH Tunnel tab content and wires it to a TunnelManager.
func buildSSHTab(win fyne.Window) *container.TabItem {
	// ── Per-tab log state ─────────────────────────────────────────────────────
	var (
		logMu    sync.Mutex
		logLines []string
	)

	logWidget := widget.NewMultiLineEntry()
	logWidget.Wrapping = fyne.TextWrapWord

	appendLog := func(msg string) {
		line := fmt.Sprintf("%s - %s", time.Now().Format("15:04:05"), msg)
		logMu.Lock()
		logLines = append(logLines, line)
		const maxLines = 200
		if len(logLines) > maxLines {
			logLines = logLines[len(logLines)-maxLines:]
		}
		text := strings.Join(logLines, "\n")
		logMu.Unlock()
		fyne.Do(func() { logWidget.SetText(text) })
	}

	// ── Status label ──────────────────────────────────────────────────────────
	statusLbl := widget.NewLabel("Status: Disconnected")
	statusLbl.Alignment = fyne.TextAlignCenter
	statusLbl.TextStyle = fyne.TextStyle{Bold: true}

	// ── Input fields ──────────────────────────────────────────────────────────
	hostEntry := widget.NewEntry()
	hostEntry.PlaceHolder = "e.g. myserver.ddns.net"

	portEntry := widget.NewEntry()
	portEntry.PlaceHolder = "e.g. 8080"

	forwardEntry := widget.NewEntry()
	forwardEntry.PlaceHolder = "e.g. 1883, 3391"

	userEntry := widget.NewEntry()
	userEntry.PlaceHolder = "SSH username"

	ipv6Check := widget.NewCheck("Force IPv6 (Required for DS-Lite)", nil)

	// ── Populate from saved config ────────────────────────────────────────────
	cfg := loadConfig()
	hostEntry.SetText(cfg.Host)
	portEntry.SetText(cfg.Port)
	forwardEntry.SetText(cfg.ForwardPorts)
	userEntry.SetText(cfg.User)
	ipv6Check.SetChecked(cfg.IPv6)

	form := widget.NewForm(
		widget.NewFormItem("Host / DNS:", hostEntry),
		widget.NewFormItem("SSH Port:", portEntry),
		widget.NewFormItem("Forward Ports (csv):", forwardEntry),
		widget.NewFormItem("SSH Username:", userEntry),
	)

	// ── Tunnel manager ────────────────────────────────────────────────────────
	tm := &TunnelManager{
		OnStatus: func(s TunnelStatus) {
			fyne.Do(func() { statusLbl.SetText(s.String()) })
		},
		OnLog: func(msg string) { appendLog(msg) },
	}

	// ── Start / Stop button ───────────────────────────────────────────────────
	var startBtn *widget.Button
	startBtn = widget.NewButtonWithIcon("START TUNNEL", theme.MediaPlayIcon(), func() {
		if tm.IsRunning() {
			tm.Stop()
			startBtn.SetText("START TUNNEL")
			startBtn.Icon = theme.MediaPlayIcon()
			startBtn.Refresh()
			enableWidgets(hostEntry, portEntry, forwardEntry, userEntry, ipv6Check)
		} else {
			host := strings.TrimSpace(hostEntry.Text)
			port := strings.TrimSpace(portEntry.Text)
			user := strings.TrimSpace(userEntry.Text)
			fwdPorts := strings.TrimSpace(forwardEntry.Text)

			var errs []string
			if host == "" {
				errs = append(errs, "• Host / DNS is required")
			}
			if port == "" {
				errs = append(errs, "• SSH Port is required")
			}
			if user == "" {
				errs = append(errs, "• SSH Username is required")
			}
			if fwdPorts == "" {
				errs = append(errs, "• At least one Forward Port is required")
			}
			if len(errs) == 0 {
				if _, err := buildSSHArgs(TunnelConfig{
					Host: host, Port: port,
					ForwardPorts: fwdPorts, User: user,
					IPv6: ipv6Check.Checked,
				}); err != nil {
					errs = append(errs, "• "+err.Error())
				}
			}
			if len(errs) > 0 {
				dialog.ShowInformation("Validation Error", strings.Join(errs, "\n"), win)
				return
			}

			saveConfig(Config{
				Host:         host,
				Port:         port,
				ForwardPorts: fwdPorts,
				User:         user,
				IPv6:         ipv6Check.Checked,
			})

			disableWidgets(hostEntry, portEntry, forwardEntry, userEntry, ipv6Check)
			startBtn.SetText("STOP TUNNEL")
			startBtn.Icon = theme.MediaStopIcon()
			startBtn.Refresh()

			tm.Start(TunnelConfig{
				Host: host, Port: port,
				ForwardPorts: fwdPorts, User: user,
				IPv6: ipv6Check.Checked,
			})
		}
	})

	appendLog("Ready. Fill in connection details and press START TUNNEL.")

	topSection := container.NewVBox(
		form,
		ipv6Check,
		widget.NewSeparator(),
		statusLbl,
		startBtn,
	)
	content := container.NewBorder(topSection, nil, nil, nil, logWidget)

	return container.NewTabItemWithIcon("SSH Tunnel", theme.ComputerIcon(), content)
}
