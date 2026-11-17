package gui

// importWindows is a host that records the URL it was asked to open; the
// tests kept from upstream use it (it lived in a removed agent's import
// test file).
type importWindows struct {
	Windows
	opened string
}

func (w *importWindows) OpenURL(u string) error { w.opened = u; return nil }
