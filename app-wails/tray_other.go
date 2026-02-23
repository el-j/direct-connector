//go:build !darwin && !windows

package main

// tray_other.go — stub for platforms that don't have a tray implementation.
// The app still hides on close (OnBeforeClose) and can be quit via the app menu.

func (a *App) setupTray()    {}
func (a *App) teardownTray() {}
