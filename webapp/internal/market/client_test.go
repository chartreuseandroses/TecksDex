package market

import (
	"encoding/json"
	"testing"
)

// TestDecodeAggregates_MixedStringAndNumberTypes reproduces the actual
// response Fuzzwork returns for a thin hub (Rens/Heimatar): most fields are
// JSON strings, but every field on a side with zero orders comes back as a
// bare JSON number 0 instead. This is the exact payload shape that broke
// region=10000030 in production with "cannot unmarshal number into Go
// struct field .sell.volume of type string".
func TestDecodeAggregates_MixedStringAndNumberTypes(t *testing.T) {
	const raw = `{
		"2073": {
			"buy": {"weightedAverage": "1.18", "max": "1.19", "min": "0.01", "stddev": "0.67", "median": "1.18", "volume": "18349456.0", "orderCount": "3", "percentile": "1.19"},
			"sell": {"weightedAverage": "4.2", "max": "4.72", "min": "2.0", "stddev": "1.9", "median": "3.36", "volume": "4463538.0", "orderCount": "2", "percentile": "2.0"}
		},
		"2305": {
			"buy": {"weightedAverage": "1.85", "max": "2.24", "min": "0.01", "stddev": "1.15", "median": "2.0", "volume": "19842791.0", "orderCount": "5", "percentile": "2.24"},
			"sell": {"weightedAverage": 0, "max": 0, "min": 0, "stddev": 0, "median": 0, "volume": 0, "orderCount": 0, "percentile": 0}
		}
	}`

	var decoded map[string]aggregateEntry
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("decode returned error: %v", err)
	}

	if got := float64(decoded["2073"].Sell.Percentile); got != 2.0 {
		t.Errorf("2073 sell percentile = %v, want 2.0", got)
	}
	if got := float64(decoded["2305"].Sell.Percentile); got != 0 {
		t.Errorf("2305 sell percentile = %v, want 0", got)
	}
	if got := float64(decoded["2305"].Buy.Percentile); got != 2.24 {
		t.Errorf("2305 buy percentile = %v, want 2.24", got)
	}
}

func TestPriceFromEntry_OneSidedZeroOrdersNotTreatedAsFreeOrWorthless(t *testing.T) {
	var entry aggregateEntry
	// Sell side (what you'd pay to acquire) has zero orders; buy side does not.
	if err := json.Unmarshal([]byte(`{
		"buy": {"percentile": "2.24", "volume": "19842791.0"},
		"sell": {"percentile": 0, "volume": 0}
	}`), &entry); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}

	price, ok := priceFromEntry(entry)
	if !ok {
		t.Fatal("expected priceFromEntry to include this typeID (buy side has data)")
	}
	if price.HasAcquireCost {
		t.Error("HasAcquireCost = true, want false: zero sell-side orders means unknown, not free")
	}
	if !price.HasDisposeValue {
		t.Error("HasDisposeValue = false, want true: buy side has real data")
	}
	if price.DisposeValue != 2.24 {
		t.Errorf("DisposeValue = %v, want 2.24", price.DisposeValue)
	}
}

func TestPriceFromEntry_BothSidesZeroIsExcluded(t *testing.T) {
	var entry aggregateEntry
	if err := json.Unmarshal([]byte(`{
		"buy": {"percentile": 0, "volume": 0},
		"sell": {"percentile": 0, "volume": 0}
	}`), &entry); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}

	if _, ok := priceFromEntry(entry); ok {
		t.Error("expected priceFromEntry to exclude a typeID with no data on either side")
	}
}
