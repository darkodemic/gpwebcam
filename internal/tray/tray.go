// Package tray shows gpwebcam's icon and menu in the desktop's system tray
// through the StatusNotifierItem D-Bus interface (fyne.io/systray), which
// KDE, Quickshell, Waybar and GNOME with the AppIndicator extension show.
// It needs only the session D-Bus, so it works from the user service.
package tray

import (
	"log/slog"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/settings"
)

// View is everything the icon and the menu show.
type View struct {
	State State
	// Status is the first line of the menu and the tooltip.
	Status   string
	Settings settings.Settings
	// Locked are settings fixed by flags of gpwebcam run; their items are
	// disabled.
	Locked map[string]bool
	// ResPending is set when the saved resolution differs from the one in
	// use; it changes only when gpwebcam restarts.
	ResPending bool
	// CanRestart shows the restart item; gpwebcam runs under systemd.
	CanRestart bool
}

// Actions are called from the tray's goroutine when the user picks an item.
type Actions struct {
	// Set changes a setting to a value.
	Set func(key, value string)
	// Restart restarts gpwebcam.
	Restart func()
}

// Tray is the icon and its menu. fyne.io/systray keeps global state, so a
// process has at most one.
type Tray struct {
	log *slog.Logger
	act Actions

	mu     sync.Mutex
	latest View
	kick   chan struct{}
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

// Start shows the icon.
func Start(log *slog.Logger, first View, act Actions) *Tray {
	t := &Tray{
		log: log, act: act, latest: first,
		kick: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}),
	}
	go t.run()
	return t
}

// Update shows a new view. Only the latest view counts; it never blocks.
func (t *Tray) Update(v View) {
	t.mu.Lock()
	t.latest = v
	t.mu.Unlock()
	select {
	case t.kick <- struct{}{}:
	default:
	}
}

// Stop removes the icon.
func (t *Tray) Stop() {
	t.once.Do(func() { close(t.stop) })
	select {
	case <-t.done:
	case <-time.After(2 * time.Second):
		t.log.Warn("tray did not stop in time")
	}
}

// click is a menu item's setting and the value picking it sets; an empty
// value flips an on/off setting. The key "restart" restarts gpwebcam.
type click struct{ key, value string }

// toggle returns the opposite of an on/off setting.
func toggle(key string, s settings.Settings) string {
	switch key {
	case "hwdec":
		if s.HWDec == "auto" {
			return "none"
		}
		return "auto"
	case "notify":
		if s.Notify {
			return "off"
		}
		return "on"
	}
	return ""
}

// items are the menu entries that change with the view.
type items struct {
	status     *systray.MenuItem
	camera     *systray.MenuItem
	cameras    map[string]*systray.MenuItem
	fov        *systray.MenuItem
	fovs       map[camera.FOV]*systray.MenuItem
	res        *systray.MenuItem
	ress       map[camera.Resolution]*systray.MenuItem
	resPending *systray.MenuItem
	hwdec      *systray.MenuItem
	notify     *systray.MenuItem
	restart    *systray.MenuItem
	hide       *systray.MenuItem
}

var (
	cameraOrder  = []string{settings.CameraDemand, settings.CameraAlways, settings.CameraOff}
	cameraLabels = map[string]string{
		settings.CameraDemand: "On demand", settings.CameraAlways: "Always on", settings.CameraOff: "Off",
	}
	fovOrder   = []camera.FOV{camera.FOVWide, camera.FOVNarrow, camera.FOVSuperView, camera.FOVLinear}
	fovLabels  = map[camera.FOV]string{camera.FOVWide: "Wide", camera.FOVNarrow: "Narrow", camera.FOVSuperView: "SuperView", camera.FOVLinear: "Linear"}
	resOrder   = []camera.Resolution{camera.Res1080, camera.Res720}
	resLabels  = map[camera.Resolution]string{camera.Res1080: "1080p", camera.Res720: "720p"}
	lockedNote = " (set by a flag of the service)"
)

func (t *Tray) run() {
	defer close(t.done)
	ready := make(chan struct{})
	start, end := systray.RunWithExternalLoop(func() { close(ready) }, func() {})
	start()
	defer end()
	select {
	case <-ready:
	case <-t.stop:
		return
	}

	clicks := make(chan click)
	forward := func(item *systray.MenuItem, c func() click) {
		go func() {
			for {
				select {
				case <-item.ClickedCh:
				case <-t.stop:
					return
				}
				select {
				case clicks <- c():
				case <-t.stop:
					return
				}
			}
		}()
	}

	systray.SetTitle("gpwebcam")
	var it items
	it.status = systray.AddMenuItem("", "")
	it.status.Disable()
	systray.AddSeparator()
	it.camera = systray.AddMenuItem("Camera", "When the GoPro streams")
	it.cameras = map[string]*systray.MenuItem{}
	for _, m := range cameraOrder {
		it.cameras[m] = it.camera.AddSubMenuItemRadio(cameraLabels[m], "", false)
		forward(it.cameras[m], func() click { return click{"camera", m} })
	}
	it.fov = systray.AddMenuItem("Field of view", "")
	it.fovs = map[camera.FOV]*systray.MenuItem{}
	for _, f := range fovOrder {
		it.fovs[f] = it.fov.AddSubMenuItemRadio(fovLabels[f], "", false)
		forward(it.fovs[f], func() click { return click{"fov", string(f)} })
	}
	it.res = systray.AddMenuItem("Resolution", "")
	it.ress = map[camera.Resolution]*systray.MenuItem{}
	for _, r := range resOrder {
		it.ress[r] = it.res.AddSubMenuItemRadio(resLabels[r], "", false)
		forward(it.ress[r], func() click { return click{"res", string(r)} })
	}
	it.resPending = it.res.AddSubMenuItem("Applies once no application uses the camera", "")
	it.resPending.Disable()
	it.hwdec = systray.AddMenuItemCheckbox("Hardware decoding", "Decode on the GPU through VAAPI when it works", false)
	forward(it.hwdec, func() click { return click{key: "hwdec"} })
	it.notify = systray.AddMenuItemCheckbox("Notifications", "", false)
	forward(it.notify, func() click { return click{key: "notify"} })
	systray.AddSeparator()
	it.restart = systray.AddMenuItem("Restart gpwebcam", "")
	forward(it.restart, func() click { return click{key: "restart"} })
	it.hide = systray.AddMenuItem("Hide icon", "Bring it back with: gpwebcam config tray on")
	forward(it.hide, func() click { return click{"tray", "off"} })

	shown := State(-1)
	for {
		t.mu.Lock()
		v := t.latest
		t.mu.Unlock()
		if v.State != shown {
			systray.SetIcon(Icon(v.State))
			shown = v.State
		}
		it.show(v)

		select {
		case <-t.kick:
		case c := <-clicks:
			if c.key == "restart" {
				t.act.Restart()
				continue
			}
			if c.value == "" {
				c.value = toggle(c.key, v.Settings)
			}
			t.act.Set(c.key, c.value)
		case <-t.stop:
			return
		}
	}
}

// show brings the menu in line with v.
func (it *items) show(v View) {
	status := v.Status
	if status == "" {
		status = "gpwebcam"
	}
	it.status.SetTitle(status)
	systray.SetTooltip("gpwebcam: " + status)

	for m, item := range it.cameras {
		check(item, v.Settings.Camera == m)
	}
	lock(it.camera, "Camera", v.Locked["camera"])
	for f, item := range it.fovs {
		check(item, v.Settings.FOV == f)
	}
	lock(it.fov, "Field of view", v.Locked["fov"])
	for r, item := range it.ress {
		check(item, v.Settings.Res == r)
	}
	lock(it.res, "Resolution", v.Locked["res"])
	if v.ResPending {
		it.resPending.Show()
	} else {
		it.resPending.Hide()
	}
	check(it.hwdec, v.Settings.HWDec == "auto")
	lock(it.hwdec, "Hardware decoding", v.Locked["hwdec"])
	check(it.notify, v.Settings.Notify)
	lock(it.notify, "Notifications", v.Locked["notify"])
	if v.CanRestart {
		it.restart.Show()
	} else {
		it.restart.Hide()
	}
	lock(it.hide, "Hide icon", v.Locked["tray"])
}

func check(item *systray.MenuItem, on bool) {
	if on {
		item.Check()
	} else {
		item.Uncheck()
	}
}

// lock disables an item whose setting a flag fixes, and says so.
func lock(item *systray.MenuItem, title string, locked bool) {
	if locked {
		item.SetTitle(title + lockedNote)
		item.Disable()
	} else {
		item.SetTitle(title)
		item.Enable()
	}
}
