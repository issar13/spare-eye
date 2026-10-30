// Package host reads what the laptop itself reports about its battery, free
// disk space and temperature, so the app can say whether it is still fit to
// keep watching.
//
// A camera that quietly dies when the battery runs out or the recordings fill
// the disk is worse than one that warned you first. Every value here is
// best-effort: sensors are missing on desktops, virtual machines and most
// non-Linux systems, so each field has an explicit "unknown" value instead of
// an error nobody could act on.
package host

// Reading is a snapshot of the machine. Zero values are never used to mean
// "unknown", because 0 is a legitimate disk size and temperature.
type Reading struct {
	// BatteryPercent is 0-100, or -1 when there is no battery or it cannot be
	// read. With several batteries it is their average.
	BatteryPercent int

	// OnMains is true when running on wall power. Desktops have no battery to
	// drain, so they report true; a laptop only reports false when the system
	// says it is discharging.
	OnMains bool

	// FreeBytes is the space available to an unprivileged user on the volume
	// holding the directory given to Read, or 0 when that cannot be read.
	FreeBytes uint64

	// CelsiusOK says whether Celsius holds a real measurement. Read it before
	// Celsius, since 0 °C is a valid temperature.
	CelsiusOK bool
	Celsius   float64
}

// noBattery is BatteryPercent's marker for "no battery, or unreadable".
const noBattery = -1

// Read takes one snapshot. The directory decides which volume FreeBytes
// describes: it should be where recordings are written, not where the program
// happens to be installed.
func Read(directory string) Reading {
	batteryPercent, onMains := readPower()
	celsius, celsiusOK := readTemperature()

	return Reading{
		BatteryPercent: batteryPercent,
		OnMains:        onMains,
		FreeBytes:      readFreeBytes(directory),
		CelsiusOK:      celsiusOK,
		Celsius:        celsius,
	}
}
