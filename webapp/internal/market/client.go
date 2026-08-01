// Package market fetches live EVE Online market prices from the Fuzzwork
// aggregates API and caches them per-region for a short TTL.
package market

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Hub is a major trade hub region offered in the UI's region selector.
type Hub struct {
	RegionID int
	Name     string
}

var Hubs = []Hub{
	{RegionID: 10000002, Name: "Jita / The Forge"},
	{RegionID: 10000043, Name: "Amarr / Domain"},
	{RegionID: 10000032, Name: "Dodixie / Sinq Laison"},
	{RegionID: 10000030, Name: "Rens / Heimatar"},
}

// Price holds what profitability calculations and market-health indicators
// need for a type: the cost to acquire one unit on the buy side, the value
// received for disposing of one unit on the sell side, and the standing
// order volume on each side (units currently on the books, not historical
// trade volume - used as a demand/supply signal since it doesn't collapse
// to a small number just because one large order filled).
//
// The two sides are independently optional: a thin hub (Rens is a common
// real example) can easily have sell orders but zero buy orders, or vice
// versa. HasAcquireCost/HasDisposeValue must be checked before trusting the
// corresponding value - a missing side is "unknown", not "free" or "worthless".
type Price struct {
	AcquireCost     float64
	HasAcquireCost  bool
	DisposeValue    float64
	HasDisposeValue bool
	BuyVolume       float64
	SellVolume      float64
}

const aggregatesURL = "https://market.fuzzwork.co.uk/aggregates/"
const cacheTTL = 10 * time.Minute

type cacheEntry struct {
	prices    map[int]Price
	fetchedAt time.Time
}

// Client fetches and caches Fuzzwork aggregates, one cache entry per region.
type Client struct {
	httpClient *http.Client
	mu         sync.Mutex
	cache      map[int]cacheEntry
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		cache:      make(map[int]cacheEntry),
	}
}

// flexFloat parses a JSON number that Fuzzwork sometimes encodes as a
// string (e.g. "1.19") and sometimes, when a side of the market has zero
// orders, as a bare JSON number (0) - a real inconsistency in their API,
// not a hypothetical one: it reproduces at thin hubs like Rens.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(data []byte) error {
	var asNumber float64
	if err := json.Unmarshal(data, &asNumber); err == nil {
		*f = flexFloat(asNumber)
		return nil
	}

	var asString string
	if err := json.Unmarshal(data, &asString); err != nil {
		return fmt.Errorf("value %s is neither a number nor a string", data)
	}
	if asString == "" {
		*f = 0
		return nil
	}
	parsed, err := strconv.ParseFloat(asString, 64)
	if err != nil {
		return fmt.Errorf("parse %q as float: %w", asString, err)
	}
	*f = flexFloat(parsed)
	return nil
}

type aggregateEntry struct {
	Buy struct {
		Percentile flexFloat `json:"percentile"`
		Volume     flexFloat `json:"volume"`
	} `json:"buy"`
	Sell struct {
		Percentile flexFloat `json:"percentile"`
		Volume     flexFloat `json:"volume"`
	} `json:"sell"`
}

// GetPrices returns Price for every requested typeID present in the live
// aggregates response. TypeIDs with no orders on one or both sides are
// omitted from the result rather than zero-filled; callers must treat a
// missing typeID as "price unknown", not "price is zero".
func (c *Client) GetPrices(regionID int, typeIDs []int) (map[int]Price, error) {
	c.mu.Lock()
	entry, ok := c.cache[regionID]
	c.mu.Unlock()

	if ok && time.Since(entry.fetchedAt) < cacheTTL {
		return entry.prices, nil
	}

	prices, err := c.fetch(regionID, typeIDs)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[regionID] = cacheEntry{prices: prices, fetchedAt: time.Now()}
	c.mu.Unlock()

	return prices, nil
}

func (c *Client) fetch(regionID int, typeIDs []int) (map[int]Price, error) {
	idStrs := make([]string, len(typeIDs))
	for i, id := range typeIDs {
		idStrs[i] = strconv.Itoa(id)
	}

	url := fmt.Sprintf("%s?region=%d&types=%s", aggregatesURL, regionID, strings.Join(idStrs, ","))

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch fuzzwork aggregates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fuzzwork aggregates returned status %d", resp.StatusCode)
	}

	var raw map[string]aggregateEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode fuzzwork aggregates: %w", err)
	}

	prices := make(map[int]Price, len(raw))
	for idStr, entry := range raw {
		typeID, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}
		if price, ok := priceFromEntry(entry); ok {
			prices[typeID] = price
		}
	}

	return prices, nil
}

// priceFromEntry converts one Fuzzwork aggregates entry into a Price. The
// second return is false when neither side has any order data at all, in
// which case the caller should omit the typeID entirely.
func priceFromEntry(entry aggregateEntry) (Price, bool) {
	hasAcquireCost := entry.Sell.Percentile != 0
	hasDisposeValue := entry.Buy.Percentile != 0
	if !hasAcquireCost && !hasDisposeValue {
		return Price{}, false
	}
	return Price{
		AcquireCost:     float64(entry.Sell.Percentile),
		HasAcquireCost:  hasAcquireCost,
		DisposeValue:    float64(entry.Buy.Percentile),
		HasDisposeValue: hasDisposeValue,
		BuyVolume:       float64(entry.Buy.Volume),
		SellVolume:      float64(entry.Sell.Volume),
	}, true
}
