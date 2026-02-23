package main

// tray.go — platform-independent tray glue: icon embedding, OnBeforeClose,
// and the three actions (open, help, quit) shared by all platform backends.

import (
	"context"
	_ "embed"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed frontend/src/assets/images/logo-universal.png
var trayIconPNG []byte

// wantsQuit is set atomically before calling runtime.Quit() so that
// beforeClose() knows to let the shutdown through instead of hiding the window.
var wantsQuit int32 // 0 = hide on close, 1 = allow quit

// beforeClose is the Wails OnBeforeClose callback.
// Returning true (prevent=true) hides the window instead of closing.
// Returning false lets the normal Wails shutdown proceed.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	if atomic.LoadInt32(&wantsQuit) == 1 {
		return false // allow Wails to quit normally
	}
	runtime.WindowHide(ctx)
	return true // swallow the close — window stays alive in the background
}

// trayOpen shows and raises the main window from the tray.
func (a *App) trayOpen() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// trayHelp shows an in-app help dialog.
func (a *App) trayHelp() {
	_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:    runtime.InfoDialog,
		Title:   "Direct Connector — Help",
		Message: "SSH Tab: encrypted port-forwarding tunnels over SSH.\nP2P Tab: serverless direct peer-to-peer via WebRTC.\n\nFirst-time setup: click \"Install SSH Key\" in the SSH tab.\nFor P2P behind symmetric NAT/CGNAT: start the built-in TURN relay before generating the offer.",
	})
}

// trayQuit marks the intent to quit and asks Wails to shut down cleanly.
func (a *App) trayQuit() {
	atomic.StoreInt32(&wantsQuit, 1)
	// Stop all active connections synchronously before asking Wails to quit.
	// OnShutdown will also call shutdown(), but calling here ensures
	// connections are closed even if Wails tears down before the callback fires.
	if a.tm != nil {
		a.tm.Stop()
	}
	a.P2PStop()
	if a.relay != nil {
		a.relay.Stop()
	}
	runtime.Quit(a.ctx)
}
