//go:build !nofyne

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
)

func main() {
	// If CLI flags or DC_MODE env var are present, run headlessly.
	if IsCLIMode() {
		runCLI()
		return
	}

	myApp := app.NewWithID("io.github.direct-connector")
	mainWindow := myApp.NewWindow("Direct Connector — SSH & P2P Tunnel")
	mainWindow.Resize(fyne.NewSize(560, 680))

	tabs := container.NewAppTabs(
		buildSSHTab(mainWindow),
		buildP2PTab(mainWindow),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	mainWindow.SetContent(tabs)
	mainWindow.ShowAndRun()
}

// ── Shared UI helpers ─────────────────────────────────────────────────────

// enableWidgets re-enables a set of Fyne Disableable widgets.
func enableWidgets(ws ...fyne.Disableable) {
	for _, w := range ws {
		w.Enable()
	}
}

// disableWidgets disables a set of Fyne Disableable widgets.
func disableWidgets(ws ...fyne.Disableable) {
	for _, w := range ws {
		w.Disable()
	}
}
