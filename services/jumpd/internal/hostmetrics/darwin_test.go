//go:build darwin

package hostmetrics

import "testing"

func TestParseVMStat(t *testing.T) {
	pageSize, availablePages, err := parseVMStat(`Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               100.
Pages active:                             200.
Pages inactive:                           300.
Pages speculative:                         25.
Pages wired down:                         400.
Pages occupied by compressor:              50.
`)
	if err != nil {
		t.Fatal(err)
	}
	if pageSize != 16384 {
		t.Fatalf("pageSize = %d, want 16384", pageSize)
	}
	if availablePages != 425 {
		t.Fatalf("availablePages = %d, want 425", availablePages)
	}
}

func TestParsePMSetBattery(t *testing.T) {
	battery, err := parsePMSetBattery(`Now drawing from 'Battery Power'
 -InternalBattery-0 (id=1234567)	87%; discharging; 4:12 remaining present: true
`)
	if err != nil {
		t.Fatal(err)
	}
	if battery == nil {
		t.Fatal("battery = nil, want status")
	}
	if battery.Percent != 87 {
		t.Fatalf("Percent = %v, want 87", battery.Percent)
	}
	if battery.State != "discharging" {
		t.Fatalf("State = %q, want discharging", battery.State)
	}
}

func TestParsePMSetBatteryNoBattery(t *testing.T) {
	battery, err := parsePMSetBattery("No batteries")
	if err != nil {
		t.Fatal(err)
	}
	if battery != nil {
		t.Fatalf("battery = %#v, want nil", battery)
	}
}
