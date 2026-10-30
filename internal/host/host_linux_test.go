//go:build linux

package host

import (
	"os"
	"path/filepath"
	"testing"
)

// useFakeSysfs points the package at a temporary tree for one test, and returns
// a function that writes an attribute into it.
func useFakeSysfs(t *testing.T) func(relativePath string, content string) {
	t.Helper()

	fakeRoot := t.TempDir()
	originalRoot := sysRoot
	sysRoot = fakeRoot
	t.Cleanup(func() { sysRoot = originalRoot })

	return func(relativePath string, content string) {
		t.Helper()

		path := filepath.Join(fakeRoot, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
}

func TestReadPower(t *testing.T) {
	tests := []struct {
		name            string
		attributes      map[string]string
		expectedPercent int
		expectedOnMains bool
	}{
		{
			name: "laptop on battery",
			attributes: map[string]string{
				"class/power_supply/BAT0/type":     "Battery",
				"class/power_supply/BAT0/capacity": "64",
				"class/power_supply/BAT0/status":   "Discharging",
				"class/power_supply/AC/type":       "Mains",
				"class/power_supply/AC/online":     "0",
			},
			expectedPercent: 64,
			expectedOnMains: false,
		},
		{
			name: "laptop charging",
			attributes: map[string]string{
				"class/power_supply/BAT0/type":     "Battery",
				"class/power_supply/BAT0/capacity": "98",
				"class/power_supply/BAT0/status":   "Charging",
				"class/power_supply/AC/type":       "Mains",
				"class/power_supply/AC/online":     "1",
			},
			expectedPercent: 98,
			expectedOnMains: true,
		},
		{
			name: "laptop with no mains supply is judged by battery status",
			attributes: map[string]string{
				"class/power_supply/BAT0/type":     "Battery",
				"class/power_supply/BAT0/capacity": "40",
				"class/power_supply/BAT0/status":   "Discharging",
			},
			expectedPercent: 40,
			expectedOnMains: false,
		},
		{
			name: "two batteries are averaged",
			attributes: map[string]string{
				"class/power_supply/BAT0/type":     "Battery",
				"class/power_supply/BAT0/capacity": "80",
				"class/power_supply/BAT0/status":   "Discharging",
				"class/power_supply/BAT1/type":     "Battery",
				"class/power_supply/BAT1/capacity": "40",
				"class/power_supply/BAT1/status":   "Discharging",
				"class/power_supply/AC/type":       "Mains",
				"class/power_supply/AC/online":     "0",
			},
			expectedPercent: 60,
			expectedOnMains: false,
		},
		{
			name: "a wireless mouse battery is ignored",
			attributes: map[string]string{
				"class/power_supply/hid-mouse/type":     "Battery",
				"class/power_supply/hid-mouse/scope":    "Device",
				"class/power_supply/hid-mouse/capacity": "10",
				"class/power_supply/hid-mouse/status":   "Discharging",
			},
			expectedPercent: -1,
			expectedOnMains: true,
		},
		{
			name: "desktop with mains only",
			attributes: map[string]string{
				"class/power_supply/AC/type":   "Mains",
				"class/power_supply/AC/online": "1",
			},
			expectedPercent: -1,
			expectedOnMains: true,
		},
		{
			name:            "desktop with no power supplies at all",
			attributes:      map[string]string{},
			expectedPercent: -1,
			expectedOnMains: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeAttribute := useFakeSysfs(t)
			for relativePath, content := range test.attributes {
				writeAttribute(relativePath, content)
			}

			batteryPercent, onMains := readPower()

			if batteryPercent != test.expectedPercent {
				t.Errorf("battery percent = %d, want %d", batteryPercent, test.expectedPercent)
			}
			if onMains != test.expectedOnMains {
				t.Errorf("on mains = %v, want %v", onMains, test.expectedOnMains)
			}
		})
	}
}

func TestReadTemperature(t *testing.T) {
	tests := []struct {
		name            string
		attributes      map[string]string
		expectedCelsius float64
		expectedOK      bool
	}{
		{
			name: "preferred zone beats acpitz regardless of directory order",
			attributes: map[string]string{
				"class/thermal/thermal_zone0/type": "acpitz",
				"class/thermal/thermal_zone0/temp": "27800",
				"class/thermal/thermal_zone1/type": "x86_pkg_temp",
				"class/thermal/thermal_zone1/temp": "52000",
			},
			expectedCelsius: 52,
			expectedOK:      true,
		},
		{
			name: "acpitz beats an unknown zone",
			attributes: map[string]string{
				"class/thermal/thermal_zone0/type": "iwlwifi_1",
				"class/thermal/thermal_zone0/temp": "35000",
				"class/thermal/thermal_zone1/type": "acpitz",
				"class/thermal/thermal_zone1/temp": "41000",
			},
			expectedCelsius: 41,
			expectedOK:      true,
		},
		{
			name: "any plausible zone is used when no preferred type exists",
			attributes: map[string]string{
				"class/thermal/thermal_zone0/type": "iwlwifi_1",
				"class/thermal/thermal_zone0/temp": "35000",
			},
			expectedCelsius: 35,
			expectedOK:      true,
		},
		{
			name: "implausible preferred zone is ignored in favour of a sane one",
			attributes: map[string]string{
				"class/thermal/thermal_zone0/type": "x86_pkg_temp",
				"class/thermal/thermal_zone0/temp": "-274000",
				"class/thermal/thermal_zone1/type": "acpitz",
				"class/thermal/thermal_zone1/temp": "44000",
			},
			expectedCelsius: 44,
			expectedOK:      true,
		},
		{
			name: "only implausible readings means unknown",
			attributes: map[string]string{
				"class/thermal/thermal_zone0/type": "acpitz",
				"class/thermal/thermal_zone0/temp": "200000",
			},
			expectedOK: false,
		},
		{
			name:       "missing thermal directory means unknown",
			attributes: map[string]string{},
			expectedOK: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeAttribute := useFakeSysfs(t)
			for relativePath, content := range test.attributes {
				writeAttribute(relativePath, content)
			}

			celsius, ok := readTemperature()

			if ok != test.expectedOK {
				t.Fatalf("ok = %v, want %v", ok, test.expectedOK)
			}
			if celsius != test.expectedCelsius {
				t.Errorf("celsius = %v, want %v", celsius, test.expectedCelsius)
			}
		})
	}
}

func TestReadFreeBytesOfARealDirectory(t *testing.T) {
	if freeBytes := readFreeBytes(t.TempDir()); freeBytes == 0 {
		t.Error("free bytes = 0 for a directory that exists")
	}
}

func TestReadFreeBytesOfAMissingDirectoryIsZero(t *testing.T) {
	if freeBytes := readFreeBytes(filepath.Join(t.TempDir(), "missing")); freeBytes != 0 {
		t.Errorf("free bytes = %d, want 0", freeBytes)
	}
}

func TestReadCombinesEveryReading(t *testing.T) {
	writeAttribute := useFakeSysfs(t)
	writeAttribute("class/power_supply/BAT0/type", "Battery")
	writeAttribute("class/power_supply/BAT0/capacity", "75")
	writeAttribute("class/power_supply/BAT0/status", "Charging")
	writeAttribute("class/thermal/thermal_zone0/type", "x86_pkg_temp")
	writeAttribute("class/thermal/thermal_zone0/temp", "41000")

	reading := Read(t.TempDir())

	if reading.BatteryPercent != 75 || !reading.OnMains || !reading.CelsiusOK || reading.Celsius != 41 || reading.FreeBytes == 0 {
		t.Errorf("unexpected reading: %+v", reading)
	}
}
