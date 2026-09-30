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
