//go:build windows

package host

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// golang.org/x/sys/windows does not wrap GetSystemPowerStatus, so it is called
// through kernel32 directly.
var getSystemPowerStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

// systemPowerStatus mirrors SYSTEM_POWER_STATUS.
type systemPowerStatus struct {
	acLineStatus        byte
	batteryFlag         byte
	batteryLifePercent  byte
	systemStatusFlag    byte
	batteryLifeTime     uint32
	batteryFullLifeTime uint32
}

const (
	acLineOnline            = 1
	acLineUnknown           = 255
	batteryFlagNoBattery    = 128
	batteryLifePercentUnset = 255
)

func readPower() (batteryPercent int, onMains bool) {
	var status systemPowerStatus
	succeeded, _, _ := getSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if succeeded == 0 {
		return noBattery, true
	}

	// 255 on the AC line means Windows does not know; treat it as wall power
	// for the same reason a desktop is, so a missing answer never alarms.
	onMains = status.acLineStatus == acLineOnline || status.acLineStatus == acLineUnknown

	if status.batteryFlag&batteryFlagNoBattery != 0 || status.batteryLifePercent == batteryLifePercentUnset {
		return noBattery, onMains
	}

	return int(status.batteryLifePercent), onMains
}

func readFreeBytes(directory string) uint64 {
	directoryName, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return 0
	}

	var freeBytesAvailable uint64
	if err := windows.GetDiskFreeSpaceEx(directoryName, &freeBytesAvailable, nil, nil); err != nil {
		return 0
	}

	return freeBytesAvailable
}

// readTemperature reports unknown. Windows exposes CPU temperature only through
// WMI thermal zones that need administrator rights and are absent on many
// machines, or through vendor drivers. The app never asks for elevation, and a
// number that is sometimes there is worse than an honest blank.
func readTemperature() (celsius float64, ok bool) { return 0, false }
