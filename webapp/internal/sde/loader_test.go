package sde

import "testing"

// realDBPath points at the actual SDE dump shipped alongside this repo.
// This is deliberately an integration test against real data, not a mock -
// the whole point is catching cases where the SDE's actual shape (or CCP's
// own numbers) doesn't match what the rest of this codebase assumes.
const realDBPath = "../../../sde/eve_sde.db"

func TestLoad_FacilityStatsMatchKnownSDEValues(t *testing.T) {
	cat, err := Load(realDBPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := map[string]FacilityStats{
		"Basic Industry Facility":    {PowerLoad: 800, CPULoad: 200},
		"Advanced Industry Facility": {PowerLoad: 700, CPULoad: 500},
		"High-Tech Production Plant": {PowerLoad: 400, CPULoad: 1100},
	}

	if len(cat.FacilityStatsByType) != len(want) {
		t.Fatalf("FacilityStatsByType has %d entries, want %d: %+v", len(cat.FacilityStatsByType), len(want), cat.FacilityStatsByType)
	}
	for facilityType, wantStats := range want {
		got, ok := cat.FacilityStatsByType[facilityType]
		if !ok {
			t.Errorf("missing facility stats for %q", facilityType)
			continue
		}
		if got != wantStats {
			t.Errorf("%s stats = %+v, want %+v", facilityType, got, wantStats)
		}
	}
}

func TestLoad_CommandCenterCapacityMatchesKnownSDEValues(t *testing.T) {
	cat, err := Load(realDBPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := map[int]CommandCenterCapacity{
		0: {PowerOutput: 6000, CPUOutput: 1675},
		1: {PowerOutput: 9000, CPUOutput: 7057},
		2: {PowerOutput: 12000, CPUOutput: 12136},
		3: {PowerOutput: 15000, CPUOutput: 17215},
		4: {PowerOutput: 17000, CPUOutput: 21315},
		5: {PowerOutput: 19000, CPUOutput: 25415},
	}

	if len(cat.CommandCenterCapacityByLevel) != len(want) {
		t.Fatalf("CommandCenterCapacityByLevel has %d entries, want %d: %+v", len(cat.CommandCenterCapacityByLevel), len(want), cat.CommandCenterCapacityByLevel)
	}
	for level, wantCapacity := range want {
		got, ok := cat.CommandCenterCapacityByLevel[level]
		if !ok {
			t.Errorf("missing command center capacity for level %d", level)
			continue
		}
		if got != wantCapacity {
			t.Errorf("level %d capacity = %+v, want %+v", level, got, wantCapacity)
		}
	}
}

func TestLoad_LinkStatsMatchKnownSDEValues(t *testing.T) {
	cat, err := Load(realDBPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := FacilityStats{PowerLoad: 10, CPULoad: 15}
	if cat.LinkStats != want {
		t.Errorf("LinkStats = %+v, want %+v", cat.LinkStats, want)
	}
}
