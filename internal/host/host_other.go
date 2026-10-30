//go:build !linux && !windows

package host

// Nothing is readable on these platforms yet. macOS would need IOKit for the
// battery and SMC for the temperature, which means cgo; this project has not
// taken that on. OnMains reports true so that a missing sensor never looks like
// a laptop about to die.

func readPower() (batteryPercent int, onMains bool) { return noBattery, true }

func readFreeBytes(string) uint64 { return 0 }

func readTemperature() (celsius float64, ok bool) { return 0, false }
