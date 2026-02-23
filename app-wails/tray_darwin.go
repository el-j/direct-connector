//go:build darwin

package main

// tray_darwin.go — macOS tray integration via a hand-rolled Obj-C CGO shim.
// We deliberately do NOT use energye/systray or getlantern/systray here because
// both libraries replace the NSApp delegate, which conflicts with Wails' own
// Cocoa delegate and breaks window-close and quit handling.
// This file calls into tray_darwin.m via CGO; that code uses dispatch_async so
// it is safe to invoke from any goroutine.

/*
#cgo CFLAGS:  -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <stdlib.h>

// Declarations only — implementations live in tray_darwin.m
void setupTrayIcon(const char *iconData, int iconLen);
void teardownTrayIcon(void);
*/
import "C"
import "unsafe"

// trayApp is the single App reference used by the exported C callbacks below.
var trayApp *App

// setupTray initialises the macOS menu-bar status item.
// Called from App.startup() — safe to call from any goroutine.
func (a *App) setupTray() {
	trayApp = a
	if len(trayIconPNG) > 0 {
		cData := C.CBytes(trayIconPNG) // malloc copy; freed after ObjC copies into NSData
		C.setupTrayIcon((*C.char)(cData), C.int(len(trayIconPNG)))
		C.free(cData) // NSData already owns a copy at this point
	} else {
		C.setupTrayIcon(nil, 0)
	}
}

// teardownTray removes the status item from the menu bar.
func (a *App) teardownTray() {
	C.teardownTrayIcon()
}

// onTrayAction is called from Objective-C when a menu item is clicked.
// action values: 0 = Open, 1 = Help, 2 = Exit
//
//export onTrayAction
func onTrayAction(action C.int) {
	if trayApp == nil {
		return
	}
	switch int(action) {
	case 0:
		trayApp.trayOpen()
	case 1:
		trayApp.trayHelp()
	case 2:
		trayApp.trayQuit()
	}
}

// keep unsafe import used
var _ = unsafe.Pointer(nil)
