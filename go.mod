module github.com/darkodemic/gpwebcam

go 1.22

require (
	fyne.io/systray v1.12.2
	github.com/godbus/dbus/v5 v5.2.2
)

require golang.org/x/sys v0.27.0 // indirect

replace fyne.io/systray => github.com/darkodemic/systray v1.12.3-0.20261006205618-9c45f672f861
