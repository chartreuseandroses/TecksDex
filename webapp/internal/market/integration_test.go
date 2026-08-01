//go:build integration

// Package market's integration tests hit the real Fuzzwork aggregates API
// over the network. They're excluded from a plain `go test ./...` (which
// should stay hermetic and fast) and only run via:
//
//	go test -tags=integration ./internal/market/...
package market

import (
	"net/http"
	"testing"
)

// TestGetPrices_LiveFuzzworkAPI sanity-checks a real response using
// order-of-magnitude, not exact-value, assertions: PI material prices in
// ISK drift substantially over time (Jita's own numbers have moved by an
// order of magnitude across EVE's history, largely tracking PLEX
// inflation), so pinning an exact price would make this test flake or rot.
// What should hold up regardless of inflation: an advanced P3 material
// costs dramatically more per unit than a cheap early-tier one, since that
// ratio comes from the production chain's own material multipliers, not
// from ISK's value.
func TestGetPrices_LiveFuzzworkAPI(t *testing.T) {
	const jita = 10000002
	const water = 3645                 // P1 material, cheap
	const highTechTransmitters = 17898 // P3 material, much pricier

	client := NewClient()
	prices, err := client.GetPrices(jita, []int{water, highTechTransmitters})
	if err != nil {
		t.Fatalf("GetPrices returned error: %v", err)
	}

	waterPrice, ok := prices[water]
	if !ok || !waterPrice.HasAcquireCost {
		t.Fatalf("no acquire price for Water (%d) - Jita should always have sell orders for it", water)
	}
	if waterPrice.AcquireCost <= 0 {
		t.Errorf("Water AcquireCost = %v, want a positive number", waterPrice.AcquireCost)
	}

	transmitterPrice, ok := prices[highTechTransmitters]
	if !ok || !transmitterPrice.HasAcquireCost {
		t.Fatalf("no acquire price for High-Tech Transmitters (%d) - Jita should always have sell orders for it", highTechTransmitters)
	}
	if transmitterPrice.AcquireCost <= 0 {
		t.Errorf("High-Tech Transmitters AcquireCost = %v, want a positive number", transmitterPrice.AcquireCost)
	}

	ratio := transmitterPrice.AcquireCost / waterPrice.AcquireCost
	if ratio < 50 {
		t.Errorf("High-Tech Transmitters/Water price ratio = %.1fx, want at least 50x (order-of-magnitude sanity check; observed ~150x at time of writing)", ratio)
	}
}

// TestGetPrices_CachesWithinTTL confirms the 10-minute cache actually
// avoids a second network round trip, using a request-counting transport
// rather than mocking time or asserting on latency.
func TestGetPrices_CachesWithinTTL(t *testing.T) {
	counter := &countingTransport{inner: http.DefaultTransport}
	client := NewClient()
	client.httpClient.Transport = counter

	const jita = 10000002
	if _, err := client.GetPrices(jita, []int{3645}); err != nil {
		t.Fatalf("first GetPrices returned error: %v", err)
	}
	if counter.count != 1 {
		t.Fatalf("expected 1 HTTP request after first call, got %d", counter.count)
	}

	if _, err := client.GetPrices(jita, []int{3645}); err != nil {
		t.Fatalf("second GetPrices returned error: %v", err)
	}
	if counter.count != 1 {
		t.Errorf("expected still 1 HTTP request after second call within the cache TTL, got %d", counter.count)
	}
}

type countingTransport struct {
	inner http.RoundTripper
	count int
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.count++
	return c.inner.RoundTrip(req)
}
