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

// newResponsiveRow wraps native controls as space or text scaling requires.
// The layout wrappers add no keyboard stops; only the controls receive focus.
func newResponsiveRow(children ...gtk.Widgetter) *gtk.FlowBox {
	row := gtk.NewFlowBox()
	row.SetSelectionMode(gtk.SelectionNone)
	row.SetFocusable(false)
	row.SetActivateOnSingleClick(false)
	row.SetMinChildrenPerLine(1)
	row.SetMaxChildrenPerLine(uint(len(children)))
	row.SetColumnSpacing(8)
	row.SetRowSpacing(8)
	row.SetHExpand(true)
	for _, widget := range children {
		child := gtk.NewFlowBoxChild()
		child.SetFocusable(false)
		child.SetChild(widget)
		row.Append(child)
	}
	return row
}

// newBrandHeader carries the same mark and type hierarchy from the launcher through
// setup, Settings and recovery. The identity remains above the scroll viewport.
func newBrandHeader() (*gtk.Box, *gtk.Label) {
	header := gtk.NewBox(gtk.OrientationHorizontal, 14)
	header.AddCSSClass("product-header")
	icon := gtk.NewImageFromIconName("com.tryomarchy.TryOmarchy")
	icon.SetPixelSize(48)
	icon.SetVAlign(gtk.AlignCenter)
	named(icon, uiText("brand.name"))
	header.Append(icon)
	identity := gtk.NewBox(gtk.OrientationVertical, 3)
	identity.SetHExpand(true)
	title := gtk.NewLabel(uiText("brand.name"))
	title.SetXAlign(0)
	title.SetWrap(true)
	title.AddCSSClass("title-1")
	title.AddCSSClass("product-title")
	identity.Append(title)
	version := gtk.NewLabel("LINUX")
	version.SetXAlign(0)
	version.AddCSSClass("caption")
	version.AddCSSClass("product-platform")
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
