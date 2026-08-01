// Package sdeupdate checks whether the local EVE SDE dump matches
// Fuzzwork's latest published build and downloads a fresh copy if not.
package sdeupdate

import (
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"teckdex/webapp/internal/sde"
)

const (
	md5URL  = "https://www.fuzzwork.co.uk/dump/latest-sqlite.db.gz.md5sum"
	dumpURL = "https://www.fuzzwork.co.uk/dump/latest-sqlite.db.gz"
)

// EnsureLatest checks dbPath's SDE dump against Fuzzwork's latest build via
// a small md5sum file (a few dozen bytes, not the ~140MB dump itself), and
// downloads + replaces it if they differ. It records the known-good md5 in
// a sidecar file (dbPath + ".md5") next to dbPath so later runs skip the
// download entirely when nothing has changed upstream.
//
// The new dump is downloaded to a temp file in the same directory as
// dbPath, its md5 verified against what Fuzzwork's own md5sum file
// promised, decompressed to another temp file, and sanity-loaded with
// sde.Load - only if all of that succeeds does it atomically replace
// dbPath. A failure at any step leaves dbPath (if it already exists)
// completely untouched.
//
// Any returned error is safe for the caller to treat as non-fatal: log it
// and continue using the existing dbPath, rather than failing startup.
func EnsureLatest(dbPath string) (updated bool, err error) {
	return ensureLatest(dbPath, md5URL, dumpURL)
}

func ensureLatest(dbPath, md5URL, dumpURL string) (updated bool, err error) {
	remoteMD5, err := fetchRemoteMD5(md5URL)
	if err != nil {
		return false, fmt.Errorf("fetch remote md5: %w", err)
	}

	sidecarPath := dbPath + ".md5"
	if _, statErr := os.Stat(dbPath); statErr == nil {
		if localMD5, readErr := os.ReadFile(sidecarPath); readErr == nil {
			if strings.TrimSpace(string(localMD5)) == remoteMD5 {
				return false, nil
			}
		}
	}

	dir := filepath.Dir(dbPath)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", dir, err)
	}

	gzPath, err := downloadDump(dumpURL, dir, remoteMD5)
	if err != nil {
		return false, fmt.Errorf("download SDE dump: %w", err)
	}
	defer os.Remove(gzPath)

	dbTempPath, err := decompressDump(gzPath, dir)
	if err != nil {
		return false, fmt.Errorf("decompress SDE dump: %w", err)
	}
	defer os.Remove(dbTempPath) // no-op once the rename below succeeds

	if _, err := sde.Load(dbTempPath); err != nil {
		return false, fmt.Errorf("sanity-load downloaded SDE dump: %w", err)
	}

	if err := os.Rename(dbTempPath, dbPath); err != nil {
		return false, fmt.Errorf("replace %s: %w", dbPath, err)
	}
	if err := os.WriteFile(sidecarPath, []byte(remoteMD5), 0o644); err != nil {
		return false, fmt.Errorf("write md5 sidecar: %w", err)
	}

	return true, nil
}

func fetchRemoteMD5(url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty md5sum response")
	}
	return fields[0], nil
}

func downloadDump(url, dir, expectedMD5 string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	out, err := os.CreateTemp(dir, "eve_sde-*.db.gz.tmp")
	if err != nil {
		return "", err
	}
	defer out.Close()

	hasher := md5.New()
	if _, err := io.Copy(io.MultiWriter(out, hasher), resp.Body); err != nil {
		os.Remove(out.Name())
		return "", err
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if got != expectedMD5 {
		os.Remove(out.Name())
		return "", fmt.Errorf("downloaded file md5 %s does not match expected %s", got, expectedMD5)
	}

	return out.Name(), nil
}

func decompressDump(gzPath, dir string) (string, error) {
	in, err := os.Open(gzPath)
	if err != nil {
		return "", err
	}
	defer in.Close()

	gz, err := gzip.NewReader(in)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	out, err := os.CreateTemp(dir, "eve_sde-*.db.tmp")
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, gz); err != nil {
		os.Remove(out.Name())
		return "", err
	}

	return out.Name(), nil
}
