package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gsk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// captureNative saves what the window draws to TRYOMARCHY_UI_CAPTURE/name.png
// when that directory is set. It renders the real widget tree through the
// window's own renderer, so the image is the frame the compositor received.
func captureNative(window *adw.ApplicationWindow, name string) error {
	dir := os.Getenv("TRYOMARCHY_UI_CAPTURE")
	if dir == "" {
		return nil
	}
	paintable := gtk.NewWidgetPaintable(window)
	width, height := float64(window.Width()), float64(window.Height())
	if width <= 0 || height <= 0 {
		return fmt.Errorf("window has no size yet")
	}
	snapshot := gtk.NewSnapshot()
	paintable.Snapshot(snapshot, width, height)
	node := snapshot.ToNode()
	if node == nil {
		return fmt.Errorf("window drew nothing")
	}
	renderer := window.Root.NativeSurface.Renderer()
	if renderer == nil {
		return fmt.Errorf("window has no renderer")
	}
	texture := gsk.BaseRenderer(renderer).RenderTexture(node, nil)
	if texture == nil {
		return fmt.Errorf("could not render the window")
	}
	path := filepath.Join(dir, name+".png")
	if !gdk.BaseTexture(texture).SaveToPNG(path) {
		return fmt.Errorf("could not save %s", path)
	}
	return nil
}
