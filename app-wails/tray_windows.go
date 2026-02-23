//go:build windows

package main

// tray_windows.go — Windows system-tray integration via energye/systray.
// On Windows, energye/systray uses a Win32 message loop in a separate goroutine
// (nativeStart), with no Cocoa involvement, so it is safe to use alongside Wails.

import (
	"github.com/energye/systray"
)

// trayApp holds the App reference for click callbacks.
var trayAppWin *App

func (a *App) setupTray() {
	trayAppWin = a

	start, _ := systray.RunWithExternalLoop(func() {
		// Windows LoadImage doesn't load raw PNG bytes so we skip the icon here;
		// the default IDI_APPLICATION icon will be used instead.
		systray.SetTooltip("Direct Connector")
		systray.SetTitle("Direct Connector")

		mOpen := systray.AddMenuItem("Open Direct Connector", "Show the main window")
		mOpen.Click(func() { trayAppWin.trayOpen() })

		systray.AddSeparator()

		mHelp := systray.AddMenuItem("Help", "")
		mHelp.Click(func() { trayAppWin.trayHelp() })

		systray.AddSeparator()

		mQuit := systray.AddMenuItem("Exit", "Quit Direct Connector")
		mQuit.Click(func() { trayAppWin.trayQuit() })

		// Left-click opens the window (Windows-only feature)
		systray.SetOnClick(func(_ systray.IMenu) { trayAppWin.trayOpen() })
	}, func() {
		// onExit — nothing to clean up
	})

	// start() launches the Win32 message loop goroutine for tray events.
	start()
}

func (a *App) teardownTray() {
	systray.Quit()
}
