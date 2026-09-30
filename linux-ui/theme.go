package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed theme.css
var brandCSS string

// The same mark and type hierarchy follow the user from the launcher through
// setup, Settings and recovery. Each page scrolls its header with its content.
func newBrandHeader() (*gtk.Box, *gtk.Label) {
	header := gtk.NewBox(gtk.OrientationHorizontal, 14)
	header.AddCSSClass("product-header")
	icon := gtk.NewImageFromIconName("com.tryomarchy.TryOmarchy")
	icon.SetPixelSize(56)
	icon.SetVAlign(gtk.AlignCenter)
	named(icon, "Try Omarchy")
	header.Append(icon)
	identity := gtk.NewBox(gtk.OrientationVertical, 3)
	identity.SetHExpand(true)
	title := gtk.NewLabel("Try Omarchy")
	title.SetXAlign(0)
	title.SetWrap(true)
	title.AddCSSClass("title-1")
	title.AddCSSClass("product-title")
	identity.Append(title)
	platform := gtk.NewLabel("OMARCHY · LINUX")
	platform.SetXAlign(0)
	platform.AddCSSClass("caption")
	platform.AddCSSClass("product-platform")
	identity.Append(platform)
	version := gtk.NewLabel("")
	version.SetXAlign(0)
	version.AddCSSClass("caption")
	version.AddCSSClass("product-version")
	identity.Append(version)
	header.Append(identity)
	return header, version
}

// Branding is scoped to our window. Native pickers and high contrast keep the
// desktop's appearance, including when high contrast changes while we are open.
func applyBrand(window *adw.ApplicationWindow) {
	provider := gtk.NewCSSProvider()
	provider.ConnectParsingError(func(_ *gtk.CSSSection, err error) {
		fmt.Fprintln(os.Stderr, "Try Omarchy style:", err)
	})
	provider.LoadFromString(brandCSS)
	gtk.StyleContextAddProviderForDisplay(window.Window.Widget.Display(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	style := adw.StyleManagerGetDefault()
	update := func() {
		if style.HighContrast() {
			window.RemoveCSSClass("try-omarchy")
		} else {
			window.AddCSSClass("try-omarchy")
		}
	}
	style.NotifyProperty("high-contrast", update)
	update()
}
