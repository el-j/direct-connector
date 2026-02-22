//go:build !nofyne

package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appconfig "mqtt-tunnel/internal/config"
	"mqtt-tunnel/internal/keygen"
	"mqtt-tunnel/internal/tunnel"
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
	portEntry.PlaceHolder = "e.g. 443  (recommended — bypasses most firewalls)"

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

	// ── Forward direction selector ────────────────────────────────────────────
	const (
		fwdRemote = "Remote (-R)  ←  remote port tunnels to this machine"
		fwdLocal  = "Local  (-L)  →  local port tunnels to remote service"
	)
	forwardModeRadio := widget.NewRadioGroup([]string{fwdRemote, fwdLocal}, nil)

	forwardModeFromRadio := func() string {
		if forwardModeRadio.Selected == fwdLocal {
			return "L"
		}
		return "R"
	}

	// ── Verbose SSH logging ───────────────────────────────────────────────────
	// Adds -v to ssh. Useful for debugging auth/forwarding; very noisy during normal use.
	verboseCheck := widget.NewCheck("Enable verbose SSH logging (-v)", nil)

	// ── WSL mode (Windows only) ─────────────────────────────────────────────
	// Runs 'wsl.exe ssh' instead of Windows ssh.exe so the tunnel runs inside
	// the WSL network namespace — the same context where Docker/MQTT lives.
	// Key path must be a Linux path (e.g. ~/.ssh/id_ed25519 or /home/user/.ssh/id_ed25519).
	wslCheck := widget.NewCheck("Use WSL ssh (Windows: run tunnel inside WSL where Docker lives)", nil)

	// ── Populate from saved config ────────────────────────────────────────────
	cfg := appconfig.Load()
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
	if cfg.ForwardMode == "L" {
		forwardModeRadio.SetSelected(fwdLocal)
	} else {
		forwardModeRadio.SetSelected(fwdRemote)
	}
	verboseCheck.SetChecked(cfg.Verbose)
	wslCheck.SetChecked(cfg.UseWSLSsh)

	form := widget.NewForm(
		widget.NewFormItem("Host / DNS:", hostEntry),
		widget.NewFormItem("SSH Port:", portEntry),
		widget.NewFormItem("Forward Ports (csv):", forwardEntry),
		widget.NewFormItem("SSH Username:", userEntry),
		widget.NewFormItem("IP Version:", ipVersionRadio),
		widget.NewFormItem("Forward Direction:", forwardModeRadio),
		widget.NewFormItem("", verboseCheck),
		widget.NewFormItem("", wslCheck),
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
			if keygen.AppKeyExists() {
				keyStatusLabel.SetText("Key: " + keygen.AppPrivateKeyPath())
				if pk, err := keygen.LoadAppPublicKey(); err == nil {
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
		if err := keygen.GenerateAppKey(); err != nil {
			dialog.ShowError(err, win)
			return
		}
		refreshKeyStatus()
		appendLog("New Ed25519 keypair generated: " + keygen.AppPrivateKeyPath())
		appendLog("Copy the public key below and add it to the server's ~/.ssh/authorized_keys")
	})

	copyPubBtn := widget.NewButtonWithIcon("Copy Public Key", theme.ContentCopyIcon(), func() {
		pk, err := keygen.LoadAppPublicKey()
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

	// ── Custom key path override ──────────────────────────────────────────────
	// Leave blank to use the app-managed key above (or SSH's default if no app key exists).
	customKeyEntry := widget.NewEntry()
	customKeyEntry.SetPlaceHolder("e.g. C:\\Users\\you\\.ssh\\id_ed25519  (leave blank to use app key)")
	if cfg.KeyPath != "" {
		customKeyEntry.SetText(cfg.KeyPath)
	}

	// Update the placeholder hint when WSL mode is toggled.
	wslCheck.OnChanged = func(checked bool) {
		if checked {
			customKeyEntry.SetPlaceHolder("WSL Linux path, e.g. /home/user/.ssh/id_ed25519  (blank = WSL default key)")
		} else {
			customKeyEntry.SetPlaceHolder("e.g. C:\\Users\\you\\.ssh\\id_ed25519  (leave blank to use app key)")
		}
	}
	if cfg.UseWSLSsh {
		wslCheck.OnChanged(true) // apply hint for restored state
	}

	browseKeyBtn := widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), func() {
		fd := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
			if err != nil || r == nil {
				return
			}
			defer r.Close()
			path := r.URI().Path()
			customKeyEntry.SetText(path)
			appendLog("Custom key path set: " + path)
		}, win)
		// Try to open the picker in the user's .ssh directory.
		if home, err := os.UserHomeDir(); err == nil {
			if lister, err := storage.ListerForURI(storage.NewFileURI(home + "/.ssh")); err == nil {
				fd.SetLocation(lister)
			}
		}
		fd.Show()
	})

	keyOverrideCard := widget.NewCard("Custom Key Path", "Override — use any existing private key instead",
		container.NewVBox(
			widget.NewLabel("Private key file (optional):"),
			container.NewBorder(nil, nil, nil, browseKeyBtn, customKeyEntry),
			widget.NewLabel("Priority: Custom path → App key → SSH default (~/.ssh/id_*)"),
		),
	)

	// ── Tunnel manager ────────────────────────────────────────────────────────
	tm := &tunnel.Manager{
		OnStatus: func(s tunnel.Status) {
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
			enableWidgets(hostEntry, portEntry, forwardEntry, userEntry, ipVersionRadio, forwardModeRadio, verboseCheck, wslCheck, customKeyEntry)
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
			fwdMode := forwardModeFromRadio()
			verbose := verboseCheck.Checked
			useWSL := wslCheck.Checked
			// Key priority: 1) custom path field  2) app-managed key  3) SSH default
			keyPath := strings.TrimSpace(customKeyEntry.Text)
			if keyPath == "" && keygen.AppKeyExists() {
				keyPath = keygen.AppPrivateKeyPath()
			}

			if len(errs) == 0 {
				if _, err := tunnel.BuildSSHArgs(tunnel.Config{
					Host: host, Port: port,
					ForwardPorts: fwdPorts, User: user,
					IPVersion: ipVer, KeyPath: keyPath,
					ForwardMode: fwdMode, Verbose: verbose, UseWSLSsh: useWSL}); err != nil {
					errs = append(errs, "• "+err.Error())
				}
			}
			if len(errs) > 0 {
				dialog.ShowInformation("Validation Error", strings.Join(errs, "\n"), win)
				return
			}

			appconfig.Save(appconfig.Config{
				Host:         host,
				Port:         port,
				ForwardPorts: fwdPorts,
				User:         user,
				IPVersion:    ipVer,
				KeyPath:      strings.TrimSpace(customKeyEntry.Text),
				ForwardMode:  fwdMode,
				Verbose:      verbose,
				UseWSLSsh:    useWSL,
			})

			disableWidgets(hostEntry, portEntry, forwardEntry, userEntry, ipVersionRadio, forwardModeRadio, verboseCheck, wslCheck, customKeyEntry)
			startBtn.SetText("STOP TUNNEL")
			startBtn.Icon = theme.MediaStopIcon()
			startBtn.Refresh()

			tm.Start(tunnel.Config{
				Host: host, Port: port,
				ForwardPorts: fwdPorts, User: user,
				IPVersion: ipVer, KeyPath: keyPath,
				ForwardMode: fwdMode, Verbose: verbose,
				UseWSLSsh: useWSL,
			})
		}
	})

	appendLog("Ready. Fill in connection details and press START TUNNEL.")

	topSection := container.NewVBox(
		form,
		widget.NewSeparator(),
		keyCard,
		keyOverrideCard,
		widget.NewSeparator(),
		statusLbl,
		startBtn,
	)
	content := container.NewBorder(topSection, nil, nil, nil, logWidget)

	return container.NewTabItemWithIcon("SSH Tunnel", theme.ComputerIcon(), content)
}
