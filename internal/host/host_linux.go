//go:build linux

package host

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// sysRoot is where the kernel publishes power and thermal sensors. It is a
// variable so tests can point it at a fake tree.
var sysRoot = "/sys"

// Zones whose readings are trusted over the rest, best first. The CPU package
// sensor is the number people mean by "how hot is it"; ACPI's is the firmware's
// own estimate and is the next most common.
var preferredThermalZoneTypes = []string{"x86_pkg_temp", "acpitz"}

// A reading outside this range is a sensor reporting nonsense (many firmwares
// return -274 or 0 for a zone they do not implement), not a machine at the
// edge of melting.
const (
	plausibleMinimumCelsius = 0
	plausibleMaximumCelsius = 120
)

type thermalZone struct {
	zoneType string
	celsius  float64
}

// readPower walks every power supply once. Wall power is any "Mains" supply
// reporting online. Some laptops expose no such supply, so in that case a
// battery that is not discharging is taken as being on mains.
func readPower() (batteryPercent int, onMains bool) {
	supplyDirectories, _ := filepath.Glob(filepath.Join(sysRoot, "class", "power_supply", "*"))

	capacities := []int{}
	mainsSeen, mainsOnline, batteryDischarging := false, false, false

	for _, supplyDirectory := range supplyDirectories {
		switch readTrimmed(filepath.Join(supplyDirectory, "type")) {
		case "Mains":
			mainsSeen = true
			mainsOnline = mainsOnline || readTrimmed(filepath.Join(supplyDirectory, "online")) == "1"
		case "Battery":
			// Wireless mice and game controllers also show up as batteries;
			// their charge says nothing about this machine.
			if readTrimmed(filepath.Join(supplyDirectory, "scope")) == "Device" {
				continue
			}
			capacity, err := strconv.Atoi(readTrimmed(filepath.Join(supplyDirectory, "capacity")))
			if err != nil {
				continue
			}
			capacities = append(capacities, capacity)
			batteryDischarging = batteryDischarging || readTrimmed(filepath.Join(supplyDirectory, "status")) == "Discharging"
		}
	}

	if len(capacities) == 0 {
		return noBattery, true
	}

	totalCapacity := 0
	for _, capacity := range capacities {
		totalCapacity += capacity
	}

	if mainsSeen {
		return totalCapacity / len(capacities), mainsOnline
	}
	return totalCapacity / len(capacities), !batteryDischarging
}

func readFreeBytes(directory string) uint64 {
	var filesystem unix.Statfs_t
	if err := unix.Statfs(directory, &filesystem); err != nil {
		return 0
	}

	// Bavail, not Bfree: the latter includes blocks reserved for root, which
	// this program can never write to.
	return filesystem.Bavail * uint64(filesystem.Bsize)
}

// readTemperature picks the preferred zone type that has a plausible reading,
// and otherwise the first plausible zone of any type.
func readTemperature() (celsius float64, ok bool) {
	zoneDirectories, _ := filepath.Glob(filepath.Join(sysRoot, "class", "thermal", "thermal_zone*"))

	zones := make([]thermalZone, 0, len(zoneDirectories))
	for _, zoneDirectory := range zoneDirectories {
		// The kernel reports thousandths of a degree.
		millidegrees, err := strconv.Atoi(readTrimmed(filepath.Join(zoneDirectory, "temp")))
		if err != nil {
			continue
		}
		zoneCelsius := float64(millidegrees) / 1000
		if zoneCelsius <= plausibleMinimumCelsius || zoneCelsius > plausibleMaximumCelsius {
			continue
		}
		zones = append(zones, thermalZone{
			zoneType: readTrimmed(filepath.Join(zoneDirectory, "type")),
			celsius:  zoneCelsius,
		})
	}

	for _, preferredType := range preferredThermalZoneTypes {
		if index := slices.IndexFunc(zones, func(zone thermalZone) bool { return zone.zoneType == preferredType }); index >= 0 {
			return zones[index].celsius, true
		}
	}

	if len(zones) == 0 {
		return 0, false
	}
	return zones[0].celsius, true
}

// readTrimmed returns a sysfs attribute without its trailing newline, or ""
// when the attribute does not exist, which is how every caller treats it.
func readTrimmed(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}
