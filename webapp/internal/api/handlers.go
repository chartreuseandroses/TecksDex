// Package api exposes the PI profitability calculator over HTTP as JSON.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"teckdex/webapp/internal/market"
	"teckdex/webapp/internal/profit"
	"teckdex/webapp/internal/sde"
)

type Server struct {
	Catalog *sde.Catalog
	Market  *market.Client
	allIDs  []int
}

func NewServer(cat *sde.Catalog, mkt *market.Client) *Server {
	ids := make([]int, 0, len(cat.Items))
	for id := range cat.Items {
		ids = append(ids, id)
	}
	return &Server{Catalog: cat, Market: mkt, allIDs: ids}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/tiers", s.handleTiers)
	mux.HandleFunc("/api/regions", s.handleRegions)
	mux.HandleFunc("/api/profitability", s.handleProfitability)
}

type tierItem struct {
	TypeID int    `json:"typeId"`
	Name   string `json:"name"`
	Demand string `json:"demand,omitempty"`
	Supply string `json:"supply,omitempty"`
}

type tierGroup struct {
	Tier  int        `json:"tier"`
	Label string     `json:"label"`
	Items []tierItem `json:"items"`
}

func (s *Server) handleTiers(w http.ResponseWriter, r *http.Request) {
	regionID := market.Hubs[0].RegionID
	if v := r.URL.Query().Get("region"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "region must be an integer regionID")
			return
		}
		regionID = parsed
	}

	prices, err := s.Market.GetPrices(regionID, s.allIDs)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("fetch market prices: %v", err))
		return
	}

	var groups []tierGroup
	for tier := sde.TierP0; tier <= sde.TierP4; tier++ {
		items := s.Catalog.ItemsByTier[tier]
		group := tierGroup{Tier: int(tier), Label: tier.String()}

		// Demand/supply is judged relative to this item's tier-mates, not
		// globally: raw P0 materials trade in vastly larger volumes than
		// advanced P4 ones, so a global comparison would just always call
		// every P0 item "heavily supplied" regardless of its actual standing.
		buyMedian := medianVolume(items, prices, func(p market.Price) float64 { return p.BuyVolume })
		sellMedian := medianVolume(items, prices, func(p market.Price) float64 { return p.SellVolume })

		for _, it := range items {
			ti := tierItem{TypeID: it.TypeID, Name: it.Name}
			if p, ok := prices[it.TypeID]; ok {
				if p.BuyVolume >= buyMedian {
					ti.Demand = "Heavily Demanded"
				} else {
					ti.Demand = "Lightly Demanded"
				}
				if p.SellVolume >= sellMedian {
					ti.Supply = "Heavily Supplied"
				} else {
					ti.Supply = "Lightly Supplied"
				}
			}
			group.Items = append(group.Items, ti)
		}
		groups = append(groups, group)
	}
	writeJSON(w, groups)
}

// medianVolume returns the median of the given volume field across items
// that have price data. Items with no market data at all are excluded so
// they don't skew the comparison for their tier-mates.
func medianVolume(items []sde.Item, prices map[int]market.Price, volume func(market.Price) float64) float64 {
	var vals []float64
	for _, it := range items {
		if p, ok := prices[it.TypeID]; ok {
			vals = append(vals, volume(p))
		}
	}
	if len(vals) == 0 {
		return 0
	}
	sort.Float64s(vals)
	mid := len(vals) / 2
	if len(vals)%2 == 0 {
		return (vals[mid-1] + vals[mid]) / 2
	}
	return vals[mid]
}

func (s *Server) handleRegions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, market.Hubs)
}

func (s *Server) handleProfitability(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	target, err := strconv.Atoi(q.Get("target"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "target must be an integer typeID")
		return
	}

	buyTierInt, err := strconv.Atoi(q.Get("buyTier"))
	if err != nil || buyTierInt < 0 || buyTierInt > 3 {
		writeError(w, http.StatusBadRequest, "buyTier must be an integer 0-3 (P0-P3)")
		return
	}

	qty := 1.0
	if v := q.Get("qty"); v != "" {
		qty, err = strconv.ParseFloat(v, 64)
		if err != nil || qty <= 0 {
			writeError(w, http.StatusBadRequest, "qty must be a positive number")
			return
		}
	}

	factories := 1
	if v := q.Get("factories"); v != "" {
		factories, err = strconv.Atoi(v)
		if err != nil || factories < 1 {
			writeError(w, http.StatusBadRequest, "factories must be a positive integer")
			return
		}
	}

	ccSkillLevel := 5
	if v := q.Get("ccSkillLevel"); v != "" {
		ccSkillLevel, err = strconv.Atoi(v)
		if err != nil || ccSkillLevel < 1 || ccSkillLevel > 5 {
			writeError(w, http.StatusBadRequest, "ccSkillLevel must be an integer 1-5")
			return
		}
	}

	regionID := market.Hubs[0].RegionID
	if v := q.Get("region"); v != "" {
		regionID, err = strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "region must be an integer regionID")
			return
		}
	}

	prices, err := s.Market.GetPrices(regionID, s.allIDs)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("fetch market prices: %v", err))
		return
	}

	res, err := profit.ComputeRollup(s.Catalog, prices, target, sde.Tier(buyTierInt), qty)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	factoryPlan, err := profit.ComputeFactoryPlan(s.Catalog, target, sde.Tier(buyTierInt), factories, qty, ccSkillLevel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, profitabilityResponse{
		Result:       res,
		ShoppingList: s.annotateShoppingList(res.ShoppingList, sde.Tier(buyTierInt), prices),
		FactoryPlan:  factoryPlan,
	})
}

// profitabilityResponse re-exposes profit.Result's fields (via embedding)
// but swaps in a shopping list annotated with demand/supply/risk, which is
// a market-classification concern that doesn't belong in the profit package.
type profitabilityResponse struct {
	*profit.Result
	ShoppingList []shoppingListItem
	FactoryPlan  *profit.FactoryPlan
}

type shoppingListItem struct {
	profit.ShoppingListEntry
	Demand string
	Supply string
	// Risk flags a shopping list item as a supply chain risk: heavily
	// demanded (competition for the same stock) and lightly supplied
	// (little standing sell-side inventory to absorb that demand).
	Risk bool
	// HasSellOrders is false when there is nothing on the market to
	// instantly buy at all (the item's UnitCost/TotalCost are then 0,
	// not a real price) - the frontend replaces the demand/supply text
	// with an explicit "no sell orders" callout in this case rather than
	// showing a misleadingly blank or zeroed-out row.
	HasSellOrders bool
	UnitVolumeM3  float64
	TotalVolumeM3 float64
}

func (s *Server) annotateShoppingList(entries []profit.ShoppingListEntry, buyTier sde.Tier, prices map[int]market.Price) []shoppingListItem {
	tierItems := s.Catalog.ItemsByTier[buyTier]
	buyMedian := medianVolume(tierItems, prices, func(p market.Price) float64 { return p.BuyVolume })
	sellMedian := medianVolume(tierItems, prices, func(p market.Price) float64 { return p.SellVolume })

	annotated := make([]shoppingListItem, len(entries))
	for i, entry := range entries {
		item := shoppingListItem{ShoppingListEntry: entry}
		item.UnitVolumeM3 = s.Catalog.Items[entry.TypeID].Volume
		item.TotalVolumeM3 = entry.Quantity * item.UnitVolumeM3
		if p, ok := prices[entry.TypeID]; ok {
			item.HasSellOrders = p.HasAcquireCost
			heavyDemand := p.BuyVolume >= buyMedian
			heavySupply := p.SellVolume >= sellMedian
			if heavyDemand {
				item.Demand = "Heavily Demanded"
			} else {
				item.Demand = "Lightly Demanded"
			}
			if heavySupply {
				item.Supply = "Heavily Supplied"
			} else {
				item.Supply = "Lightly Supplied"
			}
			item.Risk = heavyDemand && !heavySupply
		}
		annotated[i] = item
	}
	return annotated
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
