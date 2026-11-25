//go:build !darwin || !cgo

package preferences

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

func preferredLocale() string { return "" }

func SystemTimezone() string {
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" && tz != "Local" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	file, _ := filepath.EvalSymlinks("/etc/localtime")
	if _, name, ok := strings.Cut(file, "/zoneinfo/"); ok {
		return name
	}
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if name := strings.TrimSpace(string(data)); name != "" {
			return name
		}
	}
	return "UTC"
}

const (
	desktopPortal = "org.freedesktop.portal.Desktop"
	portalPath    = "/org/freedesktop/portal/desktop"
	portalAPI     = "org.freedesktop.portal.Settings"
	appearanceNS  = "org.freedesktop.appearance"
)

var portalScheme atomic.Uint32

func SystemDark() bool {
	switch portalScheme.Load() {
	case 1:
		return true
	case 2:
		return false
	default:
		return strings.Contains(strings.ToLower(os.Getenv("GTK_THEME")), "dark")
	}
}

// WatchSystemChanges subscribes to the desktop portal without blocking a
// frame or auto-starting a D-Bus daemon on a headless machine.
func WatchSystemChanges(changed func()) func() {
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	workers.Go(func() {
		address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
		if address == "" {
			dir := os.Getenv("XDG_RUNTIME_DIR")
			if dir == "" {
				return
			}
			address = "unix:path=" + dbus.EscapeBusAddressValue(filepath.Join(dir, "bus"))
		}
		bus, err := dbus.Connect(address, dbus.WithContext(ctx))
		if err != nil {
			return
		}
		defer bus.Close()
		signals := make(chan *dbus.Signal, 8)
		bus.Signal(signals)
		defer bus.RemoveSignal(signals)
		if err := bus.AddMatchSignal(dbus.WithMatchSender(desktopPortal), dbus.WithMatchInterface(portalAPI),
			dbus.WithMatchMember("SettingChanged"), dbus.WithMatchArg(0, appearanceNS), dbus.WithMatchArg(1, "color-scheme")); err != nil {
			return
		}
		update := func(value dbus.Variant) {
			if scheme, ok := value.Value().(uint32); ok && portalScheme.Swap(scheme) != scheme {
				changed()
			}
		}
		var initial map[string]map[string]dbus.Variant
		initialCtx, stopRead := context.WithTimeout(ctx, 2*time.Second)
		err = bus.Object(desktopPortal, portalPath).CallWithContext(initialCtx, portalAPI+".ReadAll", 0, []string{appearanceNS}).Store(&initial)
		stopRead()
		if err == nil {
			update(initial[appearanceNS]["color-scheme"])
		}
		for {
			select {
			case <-ctx.Done():
				return
			case signal, open := <-signals:
				if !open {
					return
				}
				if len(signal.Body) == 3 {
					if value, ok := signal.Body[2].(dbus.Variant); ok {
						update(value)
					}
				}
			}
		}
	})
	return func() {
		cancel()
		workers.Wait()
	}
}
