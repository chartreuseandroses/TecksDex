package profit

import (
	"testing"

	"teckdex/webapp/internal/market"
	"teckdex/webapp/internal/sde"
)

// buildHighTechTransmittersCatalog builds a minimal Catalog covering only
// the real High-Tech Transmitters recipe (17898 = 10x Polyaramids(2321) +
// 10x Transmitter(9840) -> 3x output), verified against the live SDE.
func buildHighTechTransmittersCatalog() *sde.Catalog {
	return &sde.Catalog{
		Items: map[int]sde.Item{
			17898: {TypeID: 17898, Name: "High-Tech Transmitters", Tier: sde.TierP3},
			2321:  {TypeID: 2321, Name: "Polyaramids", Tier: sde.TierP2},
			9840:  {TypeID: 9840, Name: "Transmitter", Tier: sde.TierP2},
		},
		SchematicForOutput: map[int]sde.Schematic{
			17898: {
				ID: 94, Name: "High-Tech Transmitter", CycleTime: 3600,
				OutputTypeID: 17898, OutputQty: 3,
				Inputs: []sde.SchematicInput{
					{TypeID: 2321, Quantity: 10},
					{TypeID: 9840, Quantity: 10},
				},
			},
		},
	}
}

func TestComputeRollup_HighTechTransmitters(t *testing.T) {
	cat := buildHighTechTransmittersCatalog()
	prices := map[int]market.Price{
		2321:  {AcquireCost: 100, HasAcquireCost: true, DisposeValue: 90, HasDisposeValue: true},
		9840:  {AcquireCost: 200, HasAcquireCost: true, DisposeValue: 180, HasDisposeValue: true},
		17898: {AcquireCost: 900, HasAcquireCost: true, DisposeValue: 500, HasDisposeValue: true},
	}

	res, err := ComputeRollup(cat, prices, 17898, sde.TierP2, 3)
	if err != nil {
		t.Fatalf("ComputeRollup returned error: %v", err)
	}

	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", res.Warnings)
	}

	if len(res.Layers) != 1 {
		t.Fatalf("expected 1 layer (P2 buy -> P3 target), got %d", len(res.Layers))
	}

	layer := res.Layers[0]
	if layer.Tier != sde.TierP3 {
		t.Errorf("layer tier = %v, want P3", layer.Tier)
	}
	if layer.Cost != 3000 {
		t.Errorf("layer cost = %v, want 3000", layer.Cost)
	}
	if layer.Value != 1500 {
		t.Errorf("layer value = %v, want 1500", layer.Value)
	}
	if layer.Profit != -1500 {
		t.Errorf("layer profit = %v, want -1500", layer.Profit)
	}

	if res.TotalCost != 3000 || res.TotalValue != 1500 || res.TotalProfit != -1500 {
		t.Errorf("totals = (%v, %v, %v), want (3000, 1500, -1500)", res.TotalCost, res.TotalValue, res.TotalProfit)
	}

	wantShopping := map[int]struct {
		qty, unitCost, total float64
	}{
		2321: {10, 100, 1000},
		9840: {10, 200, 2000},
	}
	if len(res.ShoppingList) != len(wantShopping) {
		t.Fatalf("shopping list has %d entries, want %d", len(res.ShoppingList), len(wantShopping))
	}
	for _, entry := range res.ShoppingList {
		want, ok := wantShopping[entry.TypeID]
		if !ok {
			t.Fatalf("unexpected shopping list entry for typeID %d", entry.TypeID)
		}
		if entry.Quantity != want.qty || entry.UnitCost != want.unitCost || entry.TotalCost != want.total {
			t.Errorf("shopping list entry %d = %+v, want qty=%v unitCost=%v total=%v", entry.TypeID, entry, want.qty, want.unitCost, want.total)
		}
	}
}

// buildMixedTierCatalog mimics the shape of the 3 real recipes (Nano-Factory,
// Sterile Conduits, Organic Mortar Applicators) where a P4 schematic consumes
// a lower-tier material directly, skipping the tier in between: T (P2) is
// built from B (P1, itself built from 2xA) plus a direct 1xA input that
// skips P1 entirely.
func buildMixedTierCatalog() *sde.Catalog {
	return &sde.Catalog{
		Items: map[int]sde.Item{
			0: {TypeID: 0, Name: "A", Tier: sde.TierP0},
			1: {TypeID: 1, Name: "B", Tier: sde.TierP1},
			2: {TypeID: 2, Name: "T", Tier: sde.TierP2},
		},
		SchematicForOutput: map[int]sde.Schematic{
			1: {
				ID: 1, Name: "B schematic", OutputTypeID: 1, OutputQty: 1,
				Inputs: []sde.SchematicInput{{TypeID: 0, Quantity: 2}},
			},
			2: {
				ID: 2, Name: "T schematic", OutputTypeID: 2, OutputQty: 1,
				Inputs: []sde.SchematicInput{
					{TypeID: 1, Quantity: 1},
					{TypeID: 0, Quantity: 1}, // skips P1, straight from P0
				},
			},
		},
	}
}

func TestComputeRollup_MixedTierSkipCarriesCostToFinalLayer(t *testing.T) {
	cat := buildMixedTierCatalog()
	prices := map[int]market.Price{
		0: {AcquireCost: 10, HasAcquireCost: true},
		1: {DisposeValue: 25, HasDisposeValue: true},
		2: {DisposeValue: 100, HasDisposeValue: true},
	}

	res, err := ComputeRollup(cat, prices, 2, sde.TierP0, 1)
	if err != nil {
		t.Fatalf("ComputeRollup returned error: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", res.Warnings)
	}
	if len(res.Layers) != 2 {
		t.Fatalf("expected 2 layers (P1, P2), got %d", len(res.Layers))
	}

	p1, p2 := res.Layers[0], res.Layers[1]
	if p1.Cost != 20 || p1.Value != 25 || p1.Profit != 5 {
		t.Errorf("P1 layer = %+v, want cost=20 value=25 profit=5", p1)
	}
	// The skip-tier A input must be reflected here: unitCost(T) = unitCost(B) + unitCost(A) = 20 + 10 = 30.
	// A buggy implementation that sums the *previous* tier's materials instead
	// of the current tier's would miss the direct A input and undercount this.
	if p2.Cost != 30 || p2.Value != 100 || p2.Profit != 70 {
		t.Errorf("P2 layer = %+v, want cost=30 value=100 profit=70", p2)
	}

	if res.TotalCost != 30 || res.TotalValue != 100 || res.TotalProfit != 70 {
		t.Errorf("totals = (%v, %v, %v), want (30, 100, 70)", res.TotalCost, res.TotalValue, res.TotalProfit)
	}

	// Shopping list at P0 must total 3 units of A (2 via B, 1 direct skip).
	if len(res.ShoppingList) != 1 || res.ShoppingList[0].Quantity != 3 || res.ShoppingList[0].TotalCost != 30 {
		t.Errorf("shopping list = %+v, want 1 entry of 3x A totaling 30", res.ShoppingList)
	}
}

// TestComputeRollup_OneSidedZeroOrdersTreatedAsMissingNotFree guards against
// a real bug: a thin hub (e.g. Rens) commonly has orders on one side of a
// material but zero on the other. A Price entry can exist in the map while
// still having no usable buy-side or sell-side number - that must produce a
// warning and be excluded from cost/value, not silently price the material
// at 0 (which would make it look free to acquire).
func TestComputeRollup_OneSidedZeroOrdersTreatedAsMissingNotFree(t *testing.T) {
	cat := buildHighTechTransmittersCatalog()
	prices := map[int]market.Price{
		// 2321 has real buy-side orders (sellers exist) but zero sell-side
		// orders (nobody is buying it back) - AcquireCost must still be usable.
		2321: {AcquireCost: 100, HasAcquireCost: true, HasDisposeValue: false},
		// 9840 is the inverse: no sell-side orders at all, so it cannot be
		// acquired even though a map entry exists for it.
		9840:  {HasAcquireCost: false, DisposeValue: 180, HasDisposeValue: true},
		17898: {AcquireCost: 900, HasAcquireCost: true, DisposeValue: 500, HasDisposeValue: true},
	}

	res, err := ComputeRollup(cat, prices, 17898, sde.TierP2, 1)
	if err != nil {
		t.Fatalf("ComputeRollup returned error: %v", err)
	}

	if len(res.Warnings) != 1 {
		t.Fatalf("expected exactly 1 warning about 9840's missing buy-side price, got %d: %v", len(res.Warnings), res.Warnings)
	}

	for _, entry := range res.ShoppingList {
		if entry.TypeID == 9840 && entry.UnitCost != 0 {
			t.Errorf("9840 unit cost = %v, want 0 (no sell-side orders = unknown, treated as 0 with a warning, not silently priced)", entry.UnitCost)
		}
	}
}

func TestComputeRollup_RejectsBuyTierAtOrAboveTarget(t *testing.T) {
	cat := buildHighTechTransmittersCatalog()
	prices := map[int]market.Price{}

	if _, err := ComputeRollup(cat, prices, 17898, sde.TierP3, 1); err == nil {
		t.Error("expected error when buyTier equals target tier, got nil")
	}
}

func TestComputeRollup_MissingPriceProducesWarningNotSilentZero(t *testing.T) {
	cat := buildHighTechTransmittersCatalog()
	prices := map[int]market.Price{
		2321: {AcquireCost: 100, HasAcquireCost: true, DisposeValue: 90, HasDisposeValue: true},
		// 9840 deliberately has no price entry.
		17898: {AcquireCost: 900, HasAcquireCost: true, DisposeValue: 500, HasDisposeValue: true},
	}

	res, err := ComputeRollup(cat, prices, 17898, sde.TierP2, 1)
	if err != nil {
		t.Fatalf("ComputeRollup returned error: %v", err)
	}

	if len(res.Warnings) == 0 {
		t.Error("expected a warning about the missing 9840 price, got none")
	}
}
