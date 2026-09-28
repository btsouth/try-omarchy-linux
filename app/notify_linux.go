//go:build linux

package main

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

// linuxNotify shows a desktop notification through the portal, which works
// from the sandbox on every desktop, needs no permission of its own and never
// takes focus from the Omarchy window. It is a variable so tests can watch it.
var linuxNotify = notifyThroughPortal

func notifyThroughPortal(id, title, body string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return conn.Object(linuxPortalDesktop, linuxPortalObject).CallWithContext(ctx, "org.freedesktop.portal.Notification.AddNotification", 0, id,
		map[string]dbus.Variant{
			"title":    dbus.MakeVariant(title),
			"body":     dbus.MakeVariant(body),
			"priority": dbus.MakeVariant("normal"),
		}).Err
}

// tellLinuxUser notifies and logs. A desktop without a notification service
// loses nothing but the message, which the log still has.
func tellLinuxUser(id, title, body string) {
	logf("notice: %s: %s", title, body)
	if !linuxGUIEnabled {
		return
	}
	if err := linuxNotify(id, title, body); err != nil {
		logf("notice: could not show the notification: %v", err)
	}
}
