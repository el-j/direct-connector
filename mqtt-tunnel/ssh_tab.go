//go:build !nofyne

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

	// ── IP version selector ───────────────────────────────────────────────────
	const (
		ipAuto = "Auto (let OS decide)"
		ipV4   = "Force IPv4"
		ipV6   = "Force IPv6 (DS-Lite / IPv6-only)"
	)

	ipVersionRadio := widget.NewRadioGroup([]string{ipAuto, ipV4, ipV6}, nil)

	ipVersionFromRadio := func() string {
		switch ipVersionRadio.Selected {
		case ipV4:
			return "4"
		case ipV6:
			return "6"
		default:
			return ""
		}
	}

	// ── Populate from saved config ────────────────────────────────────────────
	cfg := loadConfig()
	hostEntry.SetText(cfg.Host)
	portEntry.SetText(cfg.Port)
	forwardEntry.SetText(cfg.ForwardPorts)
	userEntry.SetText(cfg.User)
	switch cfg.IPVersion {
	case "4":
		ipVersionRadio.SetSelected(ipV4)
	case "6":
		ipVersionRadio.SetSelected(ipV6)
	default:
		ipVersionRadio.SetSelected(ipAuto)
	}

	form := widget.NewForm(
		widget.NewFormItem("Host / DNS:", hostEntry),
		widget.NewFormItem("SSH Port:", portEntry),
		widget.NewFormItem("Forward Ports (csv):", forwardEntry),
		widget.NewFormItem("SSH Username:", userEntry),
		widget.NewFormItem("IP Version:", ipVersionRadio),
	)

	// ── Key management ────────────────────────────────────────────────────────
	keyStatusLabel := widget.NewLabel("")
	keyStatusLabel.Wrapping = fyne.TextWrapBreak

	pubKeyEntry := widget.NewMultiLineEntry()
	pubKeyEntry.SetMinRowsVisible(2)
	pubKeyEntry.Wrapping = fyne.TextWrapBreak
	pubKeyEntry.Disable() // read-only display; user can still select + copy

	refreshKeyStatus := func() {
		fyne.Do(func() {
			if AppKeyExists() {
				keyStatusLabel.SetText("Key: " + appPrivateKeyPath())
				if pk, err := LoadAppPublicKey(); err == nil {
					pubKeyEntry.SetText(pk)
				}
			} else {
				keyStatusLabel.SetText("No key found — click Generate to create one.")
				pubKeyEntry.SetText("")
			}
		})
	}
	refreshKeyStatus()

	genKeyBtn := widget.NewButtonWithIcon("Generate New Key", theme.ContentAddIcon(), func() {
		if err := GenerateAppKey(); err != nil {
			dialog.ShowError(err, win)
			return
		}
		refreshKeyStatus()
		appendLog("New Ed25519 keypair generated: " + appPrivateKeyPath())
		appendLog("Copy the public key below and add it to the server's ~/.ssh/authorized_keys")
	})

	copyPubBtn := widget.NewButtonWithIcon("Copy Public Key", theme.ContentCopyIcon(), func() {
		pk, err := LoadAppPublicKey()
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		win.Clipboard().SetContent(pk)
		appendLog("Public key copied to clipboard — paste it into the server's authorized_keys.")
	})

	keyCard := widget.NewCard("SSH Key", "App-managed Ed25519 keypair",
		container.NewVBox(
			keyStatusLabel,
			container.NewGridWithColumns(2, genKeyBtn, copyPubBtn),
			widget.NewLabel("Public key (add to ~/.ssh/authorized_keys on the server):"),
			pubKeyEntry,
		),
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
			enableWidgets(hostEntry, portEntry, forwardEntry, userEntry, ipVersionRadio)
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

			ipVer := ipVersionFromRadio()
			keyPath := ""
			if AppKeyExists() {
				keyPath = appPrivateKeyPath()
			}

			if len(errs) == 0 {
				if _, err := buildSSHArgs(TunnelConfig{
					Host: host, Port: port,
					ForwardPorts: fwdPorts, User: user,
					IPVersion: ipVer, KeyPath: keyPath,
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
				IPVersion:    ipVer,
			})

			disableWidgets(hostEntry, portEntry, forwardEntry, userEntry, ipVersionRadio)
			startBtn.SetText("STOP TUNNEL")
			startBtn.Icon = theme.MediaStopIcon()
			startBtn.Refresh()

			tm.Start(TunnelConfig{
				Host: host, Port: port,
				ForwardPorts: fwdPorts, User: user,
				IPVersion: ipVer, KeyPath: keyPath,
			})
		}
	})

	appendLog("Ready. Fill in connection details and press START TUNNEL.")

	topSection := container.NewVBox(
		form,
		widget.NewSeparator(),
		keyCard,
		widget.NewSeparator(),
		statusLbl,
		startBtn,
	)
	content := container.NewBorder(topSection, nil, nil, nil, logWidget)

	return container.NewTabItemWithIcon("SSH Tunnel", theme.ComputerIcon(), content)
}
