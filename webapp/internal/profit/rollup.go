// Package profit computes Planetary Industry profitability by walking the
// SDE recipe graph and pricing it against live market data. It does no I/O
// itself; callers supply a Catalog and a price lookup.
package profit

import (
	"fmt"

	"teckdex/webapp/internal/market"
	"teckdex/webapp/internal/sde"
)

type LayerItem struct {
	TypeID    int
	Name      string
	Quantity  float64
	UnitValue float64
}

// LayerResult is the cumulative cost/value/profit of the target recipe if
// production were stopped and sold at this tier, plus the items that make
// up that tier's Value.
type LayerResult struct {
	Tier   sde.Tier
	Label  string
	Cost   float64
	Value  float64
	Profit float64
	Items  []LayerItem
}

type ShoppingListEntry struct {
	TypeID    int
	Name      string
	Quantity  float64
	UnitCost  float64
	TotalCost float64
}

type Result struct {
	TargetTypeID int
	TargetName   string
	BuyTier      sde.Tier
	BuyTierLabel string
	BatchQty     float64
	Layers       []LayerResult
	ShoppingList []ShoppingListEntry
	TotalCost    float64
	TotalValue   float64
	TotalProfit  float64
	Warnings     []string
}

// ComputeRollup computes layer-by-layer profitability for producing
// batchQty units of target, buying in raw materials at buyTier.
func ComputeRollup(cat *sde.Catalog, prices map[int]market.Price, target int, buyTier sde.Tier, batchQty float64) (*Result, error) {
	targetItem, ok := cat.Items[target]
	if !ok {
		return nil, fmt.Errorf("unknown typeID %d", target)
	}
	if buyTier >= targetItem.Tier {
		return nil, fmt.Errorf("buy tier %s must be below target tier %s", buyTier, targetItem.Tier)
	}

	tierAcc := make(map[sde.Tier]map[int]float64)
	expandTree(cat, target, 1, buyTier, tierAcc)

	memo := make(map[int]float64)
	var warnings []string

	res := &Result{
		TargetTypeID: target,
		TargetName:   targetItem.Name,
		BuyTier:      buyTier,
		BuyTierLabel: buyTier.String(),
		BatchQty:     batchQty,
	}

	for k := buyTier + 1; k <= targetItem.Tier; k++ {
		var cost, value float64

		var items []LayerItem
		for typeID, qty := range tierAcc[k] {
			item := cat.Items[typeID]
			cost += qty * unitCost(cat, prices, typeID, buyTier, memo, &warnings)

			p, ok := prices[typeID]
			if !ok || !p.HasDisposeValue {
				warnings = append(warnings, fmt.Sprintf("no market price for %s (sell)", item.Name))
				items = append(items, LayerItem{TypeID: typeID, Name: item.Name, Quantity: qty * batchQty})
				continue
			}
			value += qty * p.DisposeValue
			items = append(items, LayerItem{TypeID: typeID, Name: item.Name, Quantity: qty * batchQty, UnitValue: p.DisposeValue})
		}

		res.Layers = append(res.Layers, LayerResult{
			Tier:   k,
			Label:  k.String(),
			Cost:   cost * batchQty,
			Value:  value * batchQty,
			Profit: (value - cost) * batchQty,
			Items:  items,
		})
	}

	for typeID, qty := range tierAcc[buyTier] {
		item := cat.Items[typeID]
		p, ok := prices[typeID]
		unitCost := 0.0
		if ok && p.HasAcquireCost {
			unitCost = p.AcquireCost
		} else {
			warnings = append(warnings, fmt.Sprintf("no market price for %s (buy)", item.Name))
		}
		res.ShoppingList = append(res.ShoppingList, ShoppingListEntry{
			TypeID:    typeID,
			Name:      item.Name,
			Quantity:  qty * batchQty,
			UnitCost:  unitCost,
			TotalCost: qty * batchQty * unitCost,
		})
	}

	if len(res.Layers) > 0 {
		last := res.Layers[len(res.Layers)-1]
		res.TotalCost = last.Cost
		res.TotalValue = last.Value
		res.TotalProfit = last.Profit
	}
	// unitCost's memoized recursion and the shopping list loop below both
	// independently notice a missing price for the same typeID, so the same
	// message can be appended twice; dedupe rather than showing it twice.
	res.Warnings = dedupe(warnings)

	return res, nil
}

func dedupe(items []string) []string {
	seen := make(map[string]bool, len(items))
	var out []string
	for _, s := range items {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// unitCost returns the cost to produce (or buy, at/below buyTier) one unit
// of typeID, memoized per typeID for this buyTier.
func unitCost(cat *sde.Catalog, prices map[int]market.Price, typeID int, buyTier sde.Tier, memo map[int]float64, warnings *[]string) float64 {
	if v, ok := memo[typeID]; ok {
		return v
	}

	item := cat.Items[typeID]

	if item.Tier <= buyTier {
		p, ok := prices[typeID]
		if !ok || !p.HasAcquireCost {
			*warnings = append(*warnings, fmt.Sprintf("no market price for %s (buy)", item.Name))
			memo[typeID] = 0
			return 0
		}
		memo[typeID] = p.AcquireCost
		return p.AcquireCost
	}

	schem := cat.SchematicForOutput[typeID]
	var total float64
	for _, in := range schem.Inputs {
		total += float64(in.Quantity) * unitCost(cat, prices, in.TypeID, buyTier, memo, warnings)
	}
	cost := total / float64(schem.OutputQty)
	memo[typeID] = cost
	return cost
}

// expandTree walks the recipe graph from typeID, accumulating the quantity
// of each material needed (per 1 unit of the root target) into tierAcc,
// keyed by tier then typeID. Recursion stops once a material's tier is at
// or below buyTier, since that's where the shopping list begins.
func expandTree(cat *sde.Catalog, typeID int, qtyPerUnit float64, buyTier sde.Tier, tierAcc map[sde.Tier]map[int]float64) {
	item := cat.Items[typeID]

	if tierAcc[item.Tier] == nil {
		tierAcc[item.Tier] = make(map[int]float64)
	}
	tierAcc[item.Tier][typeID] += qtyPerUnit

	if item.Tier <= buyTier {
		return
	}

	schem := cat.SchematicForOutput[typeID]
	perOutput := qtyPerUnit / float64(schem.OutputQty)
	for _, in := range schem.Inputs {
		expandTree(cat, in.TypeID, perOutput*float64(in.Quantity), buyTier, tierAcc)
	}
}
