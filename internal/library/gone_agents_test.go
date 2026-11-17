package library

import (
	"path/filepath"
	"testing"
)

// holdApp puts a macOS app bundle, its Info.plist saying id, in the sandbox
// home's Applications, where a Mac's apps are looked for.
func holdApp(t *testing.T, h, name, id string) {
	t.Helper()
	write(t, filepath.Join(h, "Applications", name, "Contents", "Info.plist"),
		`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>`+id+`</string></dict></plist>`)
}
