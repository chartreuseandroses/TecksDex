//go:build integration

// Excluded from a plain `go test ./...` since it hits the real Fuzzwork
// servers. Run via:
//
//	go test -tags=integration ./internal/sdeupdate/...
package sdeupdate

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestFetchRemoteMD5_LiveFuzzworkAPI confirms the real md5sum endpoint still
// returns the "<hex md5>  <path>" format this package's parser expects.
func TestFetchRemoteMD5_LiveFuzzworkAPI(t *testing.T) {
	got, err := fetchRemoteMD5(md5URL)
	if err != nil {
		t.Fatalf("fetchRemoteMD5 returned error: %v", err)
	}
	if len(got) != 32 {
		t.Errorf("md5 = %q (len %d), want a 32-character hex digest", got, len(got))
	}
}

// TestDumpURL_LiveFuzzworkAPI confirms the real dump URL is reachable and
// still serves a gzip payload, without downloading the whole ~140MB body.
func TestDumpURL_LiveFuzzworkAPI(t *testing.T) {
	resp, err := http.Head(dumpURL)
	if err != nil {
		t.Fatalf("HEAD %s returned error: %v", dumpURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-gzip" {
		t.Errorf("Content-Type = %q, want application/x-gzip", ct)
	}
}

// TestEnsureLatest_SkipsWhenAlreadyCurrent runs the real flow end to end,
// but only exercises the cheap path: it seeds a sidecar with the real
// current md5 (fetched fresh) so EnsureLatest should find the local file
// already up to date and never attempt the ~140MB download.
func TestEnsureLatest_SkipsWhenAlreadyCurrent(t *testing.T) {
	currentMD5, err := fetchRemoteMD5(md5URL)
	if err != nil {
		t.Fatalf("fetchRemoteMD5 returned error: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "eve_sde.db")
	if err := os.WriteFile(dbPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("seed dbPath: %v", err)
	}
	if err := os.WriteFile(dbPath+".md5", []byte(currentMD5), 0o644); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}

	updated, err := ensureLatest(dbPath, md5URL, dumpURL)
	if err != nil {
		t.Fatalf("ensureLatest returned error: %v", err)
	}
	if updated {
		t.Error("updated = true, want false (sidecar was seeded with the current remote md5)")
	}
}
