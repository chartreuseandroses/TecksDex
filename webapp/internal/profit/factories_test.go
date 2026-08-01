package profit

import (
	"testing"

	"teckdex/webapp/internal/sde"
)

// buildChainCatalog is a straight A(P1)->B(P2)->C(P3)->D(P4) chain with the
// same cycle times as the real SDE (1800s for P1, 3600s for P2-P4) and
// quantities chosen so that, at 1 target factory, the factories-needed
// chain comes out to 2 -> 4 -> 4, matching the illustrative numbers given
// for this feature (1 P4 factory needs 2 P3, which need 4 P2, which need 4 P1).
func buildChainCatalog() *sde.Catalog {
	return &sde.Catalog{
		Items: map[int]sde.Item{
			1: {TypeID: 1, Name: "A", Tier: sde.TierP1},
			2: {TypeID: 2, Name: "B", Tier: sde.TierP2},
			3: {TypeID: 3, Name: "C", Tier: sde.TierP3},
			4: {TypeID: 4, Name: "D", Tier: sde.TierP4},
		},
		SchematicForOutput: map[int]sde.Schematic{
			1: {ID: 1, Name: "A schematic", OutputTypeID: 1, OutputQty: 20, CycleTime: 1800},
			2: {
				ID: 2, Name: "B schematic", OutputTypeID: 2, OutputQty: 1, CycleTime: 3600,
				Inputs: []sde.SchematicInput{{TypeID: 1, Quantity: 40}},
			},
			3: {
				ID: 3, Name: "C schematic", OutputTypeID: 3, OutputQty: 1, CycleTime: 3600,
				Inputs: []sde.SchematicInput{{TypeID: 2, Quantity: 2}},
			},
			4: {
				ID: 4, Name: "D schematic", OutputTypeID: 4, OutputQty: 1, CycleTime: 3600,
				Inputs: []sde.SchematicInput{{TypeID: 3, Quantity: 2}},
			},
		},
		FacilityStatsByType: map[string]sde.FacilityStats{
			"Basic Industry Facility":    {PowerLoad: 800, CPULoad: 200},
			"Advanced Industry Facility": {PowerLoad: 700, CPULoad: 500},
			"High-Tech Production Plant": {PowerLoad: 400, CPULoad: 1100},
		},
		CommandCenterCapacityByLevel: map[int]sde.CommandCenterCapacity{
			1: {PowerOutput: 9000, CPUOutput: 7057},
			2: {PowerOutput: 12000, CPUOutput: 12136},
			3: {PowerOutput: 15000, CPUOutput: 17215},
			4: {PowerOutput: 17000, CPUOutput: 21315},
			5: {PowerOutput: 19000, CPUOutput: 25415},
		},
		LinkStats: sde.FacilityStats{PowerLoad: 10, CPULoad: 15},
	}
}

func requirement(t *testing.T, plan *FactoryPlan, typeID int) FactoryRequirement {
	t.Helper()
	for _, r := range plan.Requirements {
		if r.TypeID == typeID {
			return r
		}
	}
	t.Fatalf("no requirement found for typeID %d in %+v", typeID, plan.Requirements)
	return FactoryRequirement{}
}

func TestComputeFactoryPlan_ChainedRatiosMatchIllustrativeExample(t *testing.T) {
	cat := buildChainCatalog()

	plan, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 1, 1, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}

	if got := requirement(t, plan, 3).FactoriesNeeded; got != 2 {
		t.Errorf("C (P3) factories needed = %d, want 2", got)
	}
	if got := requirement(t, plan, 2).FactoriesNeeded; got != 4 {
		t.Errorf("B (P2) factories needed = %d, want 4", got)
	}
	if got := requirement(t, plan, 1).FactoriesNeeded; got != 4 {
		t.Errorf("A (P1) factories needed = %d, want 4", got)
	}
}

func TestComputeFactoryPlan_ScalesLinearlyWithTargetFactories(t *testing.T) {
	cat := buildChainCatalog()

	plan, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 3, 1, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}

	// 3x the target factories should need 3x as many at every tier below.
	if got := requirement(t, plan, 3).FactoriesNeeded; got != 6 {
		t.Errorf("C (P3) factories needed = %d, want 6", got)
	}
	if got := requirement(t, plan, 2).FactoriesNeeded; got != 12 {
		t.Errorf("B (P2) factories needed = %d, want 12", got)
	}
	if got := requirement(t, plan, 1).FactoriesNeeded; got != 12 {
		t.Errorf("A (P1) factories needed = %d, want 12", got)
	}
}

// buildSkipCatalog mirrors the real Nano-Factory shape: a P4 target that
// consumes a P1 material directly, skipping P2 and P3 entirely - using the
// real SDE's own numbers (P1: 20 units per 30 min cycle = 40/hour) to
// confirm the rate-based formula needs no special-casing for this case.
func buildSkipCatalog() *sde.Catalog {
	return &sde.Catalog{
		Items: map[int]sde.Item{
			1: {TypeID: 1, Name: "ReactiveMetalsLike", Tier: sde.TierP1},
			4: {TypeID: 4, Name: "NanoFactoryLike", Tier: sde.TierP4},
		},
		SchematicForOutput: map[int]sde.Schematic{
			1: {ID: 1, Name: "P1 schematic", OutputTypeID: 1, OutputQty: 20, CycleTime: 1800},
			4: {
				ID: 4, Name: "P4 schematic", OutputTypeID: 4, OutputQty: 1, CycleTime: 3600,
				Inputs: []sde.SchematicInput{{TypeID: 1, Quantity: 40}},
			},
		},
		CommandCenterCapacityByLevel: map[int]sde.CommandCenterCapacity{
			5: {PowerOutput: 19000, CPUOutput: 25415},
		},
	}
}

func TestComputeFactoryPlan_SkipTierNeedsNoSpecialCasing(t *testing.T) {
	cat := buildSkipCatalog()

	plan, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 1, 1, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}

	// Requirements holds the target itself plus the one skip-tier P1 material.
	if len(plan.Requirements) != 2 {
		t.Fatalf("expected 2 requirements (target + skip-tier P1 material), got %d: %+v", len(plan.Requirements), plan.Requirements)
	}
	if got := requirement(t, plan, 1).FactoriesNeeded; got != 1 {
		t.Errorf("skip-tier P1 factories needed = %d, want 1 (40 needed/hr against 40 produced/hr)", got)
	}
}

func TestComputeFactoryPlan_PowerAndCPULoadPropagateAndSum(t *testing.T) {
	cat := buildChainCatalog()

	plan, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 1, 1, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}

	d, c, b, a := requirement(t, plan, 4), requirement(t, plan, 3), requirement(t, plan, 2), requirement(t, plan, 1)

	cases := []struct {
		name                         string
		req                          FactoryRequirement
		wantPower, wantCPU           float64
		wantTotalPower, wantTotalCPU float64
	}{
		{"D (P4, 1 factory)", d, 400, 1100, 400, 1100},
		{"C (P3, 2 factories)", c, 700, 500, 1400, 1000},
		{"B (P2, 4 factories)", b, 700, 500, 2800, 2000},
		{"A (P1, 4 factories)", a, 800, 200, 3200, 800},
	}
	for _, tc := range cases {
		if tc.req.PowerLoad != tc.wantPower || tc.req.CPULoad != tc.wantCPU {
			t.Errorf("%s: PowerLoad/CPULoad = %v/%v, want %v/%v", tc.name, tc.req.PowerLoad, tc.req.CPULoad, tc.wantPower, tc.wantCPU)
		}
		if tc.req.TotalPowerLoad != tc.wantTotalPower || tc.req.TotalCPULoad != tc.wantTotalCPU {
			t.Errorf("%s: TotalPowerLoad/TotalCPULoad = %v/%v, want %v/%v", tc.name, tc.req.TotalPowerLoad, tc.req.TotalCPULoad, tc.wantTotalPower, tc.wantTotalCPU)
		}
	}

	if plan.TotalPowerLoad != 7800 {
		t.Errorf("plan.TotalPowerLoad = %v, want 7800", plan.TotalPowerLoad)
	}
	if plan.TotalCPULoad != 4900 {
		t.Errorf("plan.TotalCPULoad = %v, want 4900", plan.TotalCPULoad)
	}
}

func TestComputeFactoryPlan_TargetRowIsLastMatchingLayerTableFlow(t *testing.T) {
	cat := buildChainCatalog()

	plan, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 5, 1, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}

	if len(plan.Requirements) == 0 {
		t.Fatal("expected at least one requirement")
	}
	if got := plan.Requirements[0].Tier; got != sde.TierP1 {
		t.Errorf("first row tier = %v, want P1 (lowest tier first)", got)
	}
	last := plan.Requirements[len(plan.Requirements)-1]
	if last.TypeID != 4 {
		t.Errorf("last row TypeID = %d, want 4 (the target)", last.TypeID)
	}
	if last.FactoriesNeeded != 5 {
		t.Errorf("target row FactoriesNeeded = %d, want 5 (the entered target factory count)", last.FactoriesNeeded)
	}
	if last.FacilityType != "High-Tech Production Plant" {
		t.Errorf("target (P4) FacilityType = %q, want %q", last.FacilityType, "High-Tech Production Plant")
	}
}

func TestFacilityTypeForTier(t *testing.T) {
	cases := map[sde.Tier]string{
		sde.TierP1: "Basic Industry Facility",
		sde.TierP2: "Advanced Industry Facility",
		sde.TierP3: "Advanced Industry Facility",
		sde.TierP4: "High-Tech Production Plant",
	}
	for tier, want := range cases {
		if got := facilityTypeForTier(tier); got != want {
			t.Errorf("facilityTypeForTier(%v) = %q, want %q", tier, got, want)
		}
	}
}

func TestComputeFactoryPlan_RejectsBuyTierAtOrAboveTarget(t *testing.T) {
	cat := buildChainCatalog()

	if _, err := ComputeFactoryPlan(cat, 4, sde.TierP4, 1, 1, 5); err == nil {
		t.Error("expected error when buyTier equals target tier, got nil")
	}
}

func TestComputeFactoryPlan_RejectsZeroOrNegativeFactories(t *testing.T) {
	cat := buildChainCatalog()

	if _, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 0, 1, 5); err == nil {
		t.Error("expected error for 0 target factories, got nil")
	}
	if _, err := ComputeFactoryPlan(cat, 4, sde.TierP0, -1, 1, 5); err == nil {
		t.Error("expected error for negative target factories, got nil")
	}
}

func TestComputeFactoryPlan_RejectsSkillLevelOutOfRange(t *testing.T) {
	cat := buildChainCatalog()

	if _, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 1, 1, 0); err == nil {
		t.Error("expected error for command center skill level 0, got nil")
	}
	if _, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 1, 1, 6); err == nil {
		t.Error("expected error for command center skill level 6, got nil")
	}
}

func TestComputeFactoryPlan_MinLinksIsOneForASingleBuilding(t *testing.T) {
	cat := buildChainCatalog()

	// buyTier = P3 means only the target (D) itself is produced locally;
	// everything it needs is bought at P3, so there's only 1 building - but
	// it still needs 1 link to reach the Command Center/Launchpad hub.
	plan, err := ComputeFactoryPlan(cat, 4, sde.TierP3, 1, 1, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}
	if plan.TotalFactories != 1 {
		t.Fatalf("TotalFactories = %d, want 1", plan.TotalFactories)
	}
	if plan.MinLinks != 1 {
		t.Errorf("MinLinks = %d, want 1 (even a single building must link to the hub)", plan.MinLinks)
	}
	if plan.TotalLinkPowerLoad != 10 || plan.TotalLinkCPULoad != 15 {
		t.Errorf("TotalLinkPowerLoad/TotalLinkCPULoad = %v/%v, want 10/15", plan.TotalLinkPowerLoad, plan.TotalLinkCPULoad)
	}
}

func TestComputeFactoryPlan_MinLinksAndMinCommandCenters(t *testing.T) {
	cat := buildChainCatalog()

	// 1 target factory: FactoriesNeeded are D=1, C=2, B=4, A=4 -> 11 buildings.
	// Plus the Command Center/Launchpad hub, that's 12 nodes, so a spanning
	// tree needs 11 links: 11*10=110 power, 11*15=165 CPU. Combined with the
	// factories' own 7800/4900, that's 7910/5065 - well under a level 5
	// Command Center's 19000/25415, so 1 CC suffices.
	small, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 1, 1, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}
	if small.TotalFactories != 11 {
		t.Errorf("TotalFactories = %d, want 11", small.TotalFactories)
	}
	if small.MinLinks != 11 {
		t.Errorf("MinLinks = %d, want 11", small.MinLinks)
	}
	if small.TotalLinkPowerLoad != 110 || small.TotalLinkCPULoad != 165 {
		t.Errorf("TotalLinkPowerLoad/TotalLinkCPULoad = %v/%v, want 110/165", small.TotalLinkPowerLoad, small.TotalLinkCPULoad)
	}
	if small.MinCommandCenters != 1 {
		t.Errorf("MinCommandCenters = %d, want 1", small.MinCommandCenters)
	}

	// 3 target factories triples every FactoriesNeeded (33 buildings, 33
	// links including the hub: 330 power, 495 CPU), and the factories' own
	// totals triple too (23400/14700). Combined (23730/15195) against a
	// level 1 CC's 9000/7057 needs 3 on both axes (ceil(23730/9000)=3,
	// ceil(15195/7057)=3).
	big, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 3, 1, 1)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}
	if big.TotalFactories != 33 {
		t.Errorf("TotalFactories = %d, want 33", big.TotalFactories)
	}
	if big.MinLinks != 33 {
		t.Errorf("MinLinks = %d, want 33", big.MinLinks)
	}
	if big.MinCommandCenters != 3 {
		t.Errorf("MinCommandCenters = %d, want 3", big.MinCommandCenters)
	}
}

func TestComputeFactoryPlan_BuildTimeRoundsUpToWholeCycles(t *testing.T) {
	cat := buildChainCatalog() // D's schematic: OutputQty=1, CycleTime=3600s (60 min)

	// 2 factories x 1 unit/cycle = 2 units/cycle. 250 units needs ceil(250/2)=125 cycles.
	plan, err := ComputeFactoryPlan(cat, 4, sde.TierP0, 2, 250, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}
	if plan.BuildTimeMinutes != 125*60 {
		t.Errorf("BuildTimeMinutes = %d, want %d", plan.BuildTimeMinutes, 125*60)
	}

	// A batch smaller than one cycle's output still takes a full cycle.
	plan, err = ComputeFactoryPlan(cat, 4, sde.TierP0, 2, 1, 5)
	if err != nil {
		t.Fatalf("ComputeFactoryPlan returned error: %v", err)
	}
	if plan.BuildTimeMinutes != 60 {
		t.Errorf("BuildTimeMinutes = %d, want 60 (rounds up to 1 cycle)", plan.BuildTimeMinutes)
	}
}
