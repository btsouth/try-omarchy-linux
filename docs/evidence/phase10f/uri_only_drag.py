#!/usr/bin/python3
"""Private GTK drag source that offers only a file URI, without a portal key."""

import sys
from pathlib import Path
from urllib.parse import quote

import gi

gi.require_version("Gtk", "4.0")
from gi.repository import Gdk, GLib, Gtk


source_path = Path(sys.argv[1]).resolve()
uri = "file://" + quote(str(source_path), safe="/") + "\r\n"
provider = Gdk.ContentProvider.new_for_bytes(
    "text/uri-list", GLib.Bytes.new(uri.encode("utf-8"))
)
app = Gtk.Application()


def activate(application):
    window = Gtk.ApplicationWindow(application=application)
    window.set_title("Private file URI drag fixture")
    window.set_default_size(460, 180)
    label = Gtk.Label(label="Drag this file URI onto Try Omarchy")
    source = Gtk.DragSource.new()
    source.set_actions(Gdk.DragAction.COPY)
    source.set_content(provider)
    label.add_controller(source)
    window.set_child(label)
    window.present()


app.connect("activate", activate)
app.run(None)
