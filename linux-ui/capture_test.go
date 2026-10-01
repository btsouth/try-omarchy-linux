package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/graphene"
	"github.com/diamondburned/gotk4/pkg/gsk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// captureNative saves what the window draws to TRYOMARCHY_UI_CAPTURE/name.png
// when that directory is set. It renders the real widget tree through the
// window's own renderer, so the image is the frame the compositor received.
func captureNative(window *adw.ApplicationWindow, name string) (err error) {
	dir := os.Getenv("TRYOMARCHY_UI_CAPTURE")
	if dir == "" {
		return nil
	}
	defer func() {
		// The bindings panic when GTK returns no texture; a capture is
		// evidence, not a check, so report it instead of failing the run.
		if r := recover(); r != nil {
			err = fmt.Errorf("render failed: %v", r)
		}
	}()
	width, height := float32(window.Width()), float32(window.Height())
	if width <= 0 || height <= 0 {
		return fmt.Errorf("window has no size yet")
	}
	paintable := gtk.NewWidgetPaintable(window)
	snapshot := gtk.NewSnapshot()
	paintable.Snapshot(snapshot, float64(width), float64(height))
	node := snapshot.ToNode()
	if node == nil {
		return fmt.Errorf("window drew nothing")
	}
	renderer := gsk.NewCairoRenderer()
	if err := renderer.RealizeForDisplay(gdk.DisplayGetDefault()); err != nil {
		return err
	}
	defer renderer.Unrealize()
	viewport := graphene.RectAlloc().Init(0, 0, width, height)
	texture := renderer.RenderTexture(node, viewport)
	path := filepath.Join(dir, name+".png")
	if !gdk.BaseTexture(texture).SaveToPNG(path) {
		return fmt.Errorf("could not save %s", path)
	}
	return nil
}
