//go:build linux

package main

import (
	"context"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

var linuxSettingsRequests = make(chan struct{}, 1)
var linuxShutdownRequests = make(chan struct{}, 1)

func requestTraySettings() bool {
	if !linuxGUIEnabled {
		return false
	}
	select {
	case linuxSettingsRequests <- struct{}{}:
	default:
	}
	return true
}

type linuxTray struct{}

func (*linuxTray) Activate(x, y int32) *dbus.Error                    { requestTraySettings(); return nil }
func (*linuxTray) SecondaryActivate(x, y int32) *dbus.Error           { requestTraySettings(); return nil }
func (*linuxTray) ContextMenu(x, y int32) *dbus.Error                 { requestTraySettings(); return nil }
func (*linuxTray) Scroll(delta int32, orientation string) *dbus.Error { return nil }

type linuxTrayMenu struct{}
type linuxMenuNode struct {
	ID         int32
	Properties map[string]dbus.Variant
	Children   []dbus.Variant
}

// linuxTrayLayout describes the tray menu for dbusmenu hosts: Settings,
// Reclaim disk space and Shut down.
func linuxTrayLayout(id int32) linuxMenuNode {
	n := linuxMenuNode{ID: id, Properties: map[string]dbus.Variant{}, Children: []dbus.Variant{}}
	labels := map[int32]string{1: "Settings...", 3: "Reclaim disk space...", 2: "Shut down Omarchy..."}
	if label, ok := labels[id]; ok {
		n.Properties = map[string]dbus.Variant{"label": dbus.MakeVariant(label), "enabled": dbus.MakeVariant(true), "visible": dbus.MakeVariant(true)}
	} else if id == 0 {
		n.Properties["children-display"] = dbus.MakeVariant("submenu")
		for _, child := range []int32{1, 3, 2} {
			n.Children = append(n.Children, dbus.MakeVariant(linuxTrayLayout(child)))
		}
	}
	return n
}
func (*linuxTrayMenu) GetLayout(id, depth int32, names []string) (uint32, linuxMenuNode, *dbus.Error) {
	return 1, linuxTrayLayout(id), nil
}
func (*linuxTrayMenu) AboutToShow(id int32) (bool, *dbus.Error) { return false, nil }

// Event dispatches clicked menu items; hovering and other events do nothing.
func (*linuxTrayMenu) Event(id int32, event string, data dbus.Variant, timestamp uint32) *dbus.Error {
	if event == "clicked" {
		switch id {
		case 1:
			requestTraySettings()
		case 3:
			requestTrayReclaim()
		case 2:
			select {
			case linuxShutdownRequests <- struct{}{}:
			default:
			}
		}
	}
	return nil
}
func (*linuxTrayMenu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	v, ok := linuxTrayLayout(id).Properties[name]
	if !ok {
		return dbus.Variant{}, dbus.NewError("com.canonical.dbusmenu.Error.PropertyNotFound", []any{name})
	}
	return v, nil
}

type linuxMenuProperties struct {
	ID         int32
	Properties map[string]dbus.Variant
}

func (*linuxTrayMenu) GetGroupProperties(ids []int32, names []string) ([]linuxMenuProperties, *dbus.Error) {
	if len(ids) == 0 {
		ids = []int32{0, 1, 2}
	}
	out := make([]linuxMenuProperties, 0, len(ids))
	for _, id := range ids {
		out = append(out, linuxMenuProperties{id, linuxTrayLayout(id).Properties})
	}
	return out, nil
}

func startLinuxTray() func() {
	if !linuxGUIEnabled || os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return func() {}
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		logf("tray: %v", err)
		return func() {}
	}
	name := linuxAppID + ".Tray"
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return func() {}
	}
	item, menu := &linuxTray{}, &linuxTrayMenu{}
	itemPath, menuPath := dbus.ObjectPath("/StatusNotifierItem"), dbus.ObjectPath("/MenuBar")
	if err := conn.Export(item, itemPath, "org.kde.StatusNotifierItem"); err != nil {
		conn.Close()
		return func() {}
	}
	if err := conn.Export(menu, menuPath, "com.canonical.dbusmenu"); err != nil {
		conn.Close()
		return func() {}
	}
	for _, entry := range []struct {
		path  dbus.ObjectPath
		iface string
		value any
	}{{itemPath, "org.kde.StatusNotifierItem", item}, {menuPath, "com.canonical.dbusmenu", menu}} {
		conn.Export(introspect.NewIntrospectable(&introspect.Node{Interfaces: []introspect.Interface{{Name: entry.iface, Methods: introspect.Methods(entry.value)}, prop.IntrospectData}}), entry.path, "org.freedesktop.DBus.Introspectable")
	}
	props := map[string]*prop.Prop{}
	for key, value := range map[string]any{"Category": "ApplicationStatus", "Id": linuxAppID, "Title": appTitle, "Status": "Active", "IconName": linuxAppID, "ItemIsMenu": true, "Menu": menuPath} {
		props[key] = &prop.Prop{Value: value, Writable: false, Emit: prop.EmitConst}
	}
	prop.Export(conn, itemPath, map[string]map[string]*prop.Prop{"org.kde.StatusNotifierItem": props})
	prop.Export(conn, menuPath, map[string]map[string]*prop.Prop{"com.canonical.dbusmenu": {"Version": {Value: uint32(3)}, "TextDirection": {Value: "ltr"}, "Status": {Value: "normal"}, "IconThemePath": {Value: []string{}}}})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		owner := ""
		for {
			callCtx, done := context.WithTimeout(ctx, 2*time.Second)
			var next string
			err := conn.BusObject().CallWithContext(callCtx, "org.freedesktop.DBus.GetNameOwner", 0, "org.kde.StatusNotifierWatcher").Store(&next)
			if err == nil && next != owner {
				if err = conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").CallWithContext(callCtx, "org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, name).Err; err == nil {
					owner = next
					logf("tray: registered")
				}
			} else if err != nil {
				owner = ""
			}
			done()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); conn.Close() }
}
