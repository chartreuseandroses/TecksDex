package sdeupdate

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildFixtureDump creates a minimal SQLite database - just enough for
// sde.Load to succeed (it requires at least the Link item's power/CPU
// attributes to exist; every other table is fine empty) - and returns it
// gzip-compressed along with the compressed bytes' md5 hex digest. Using a
// real (if tiny) SQLite file here, rather than mocking sde.Load, is what
// lets these tests actually exercise the sanity-load step.
func buildFixtureDump(t *testing.T) (gz []byte, md5Hex string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "fixture.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	stmts := []string{
		`CREATE TABLE invTypes (typeID INTEGER, typeName TEXT, description TEXT, marketGroupID INTEGER, volume REAL, groupID INTEGER)`,
		`CREATE TABLE planetSchematics (schematicID INTEGER, schematicName TEXT, cycleTime INTEGER)`,
		`CREATE TABLE planetSchematicsTypeMap (schematicID INTEGER, typeID INTEGER, quantity INTEGER, isInput INTEGER)`,
		`CREATE TABLE dgmTypeAttributes (typeID INTEGER, attributeID INTEGER, valueInt INTEGER, valueFloat REAL)`,
		`INSERT INTO invTypes (typeID, typeName, description, marketGroupID, volume, groupID) VALUES (2280, 'Link', '', NULL, 0, 1036)`,
		`INSERT INTO dgmTypeAttributes (typeID, attributeID, valueFloat) VALUES (2280, 15, 10.0)`,
		`INSERT INTO dgmTypeAttributes (typeID, attributeID, valueFloat) VALUES (2280, 49, 15.0)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}

	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read fixture db: %v", err)
	}

	var buf bytes.Buffer
	gw, err := gzip.NewWriterLevel(&buf, gzip.NoCompression)
	if err != nil {
		t.Fatalf("new gzip writer: %v", err)
	}
	if _, err := gw.Write(raw); err != nil {
		t.Fatalf("gzip fixture db: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	sum := md5.Sum(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

// fixtureServer serves a gzipped SDE dump and its md5sum at the given
// bytes/digest, tracking how many times the (large) dump endpoint was hit
// so tests can assert a cached run never re-downloads it.
type fixtureServer struct {
	*httptest.Server
	dumpHits *int
}

func startFixtureServer(t *testing.T, gz []byte, md5Hex string) fixtureServer {
	t.Helper()
	hits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/latest-sqlite.db.gz.md5sum", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  /home/web/fuzzwork/htdocs/dump/latest-sqlite.db.gz\n", md5Hex)
	})
	mux.HandleFunc("/latest-sqlite.db.gz", func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write(gz)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return fixtureServer{Server: srv, dumpHits: &hits}
}

func (s fixtureServer) md5URL() string  { return s.URL + "/latest-sqlite.db.gz.md5sum" }
func (s fixtureServer) dumpURL() string { return s.URL + "/latest-sqlite.db.gz" }

func TestEnsureLatest_DownloadsWhenNoLocalFile(t *testing.T) {
	gz, md5Hex := buildFixtureDump(t)
	srv := startFixtureServer(t, gz, md5Hex)

	dbPath := filepath.Join(t.TempDir(), "eve_sde.db")

	updated, err := ensureLatest(dbPath, srv.md5URL(), srv.dumpURL())
	if err != nil {
		t.Fatalf("ensureLatest returned error: %v", err)
	}
	if !updated {
		t.Error("updated = false, want true (no local file existed)")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("dbPath does not exist after update: %v", err)
	}
	sidecar, err := os.ReadFile(dbPath + ".md5")
	if err != nil || string(sidecar) != md5Hex {
		t.Errorf("sidecar = %q (err=%v), want %q", sidecar, err, md5Hex)
	}
	if *srv.dumpHits != 1 {
		t.Errorf("dump endpoint hit %d times, want 1", *srv.dumpHits)
	}
}

func TestEnsureLatest_SkipsDownloadWhenSidecarMatches(t *testing.T) {
	gz, md5Hex := buildFixtureDump(t)
	srv := startFixtureServer(t, gz, md5Hex)

	dbPath := filepath.Join(t.TempDir(), "eve_sde.db")
	if err := os.WriteFile(dbPath, []byte("existing content, should not change"), 0o644); err != nil {
		t.Fatalf("seed dbPath: %v", err)
	}
	if err := os.WriteFile(dbPath+".md5", []byte(md5Hex), 0o644); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}

	updated, err := ensureLatest(dbPath, srv.md5URL(), srv.dumpURL())
	if err != nil {
		t.Fatalf("ensureLatest returned error: %v", err)
	}
	if updated {
		t.Error("updated = true, want false (sidecar already matched remote md5)")
	}
	if *srv.dumpHits != 0 {
		t.Errorf("dump endpoint hit %d times, want 0 (should not download when up to date)", *srv.dumpHits)
	}
	content, _ := os.ReadFile(dbPath)
	if string(content) != "existing content, should not change" {
		t.Error("dbPath was modified despite being up to date")
	}
}

func TestEnsureLatest_UpdatesWhenSidecarMismatches(t *testing.T) {
	gz, md5Hex := buildFixtureDump(t)
	srv := startFixtureServer(t, gz, md5Hex)

	dbPath := filepath.Join(t.TempDir(), "eve_sde.db")
	if err := os.WriteFile(dbPath, []byte("stale content"), 0o644); err != nil {
		t.Fatalf("seed dbPath: %v", err)
	}
	if err := os.WriteFile(dbPath+".md5", []byte("0000stale0000000000000000000000"), 0o644); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}

	updated, err := ensureLatest(dbPath, srv.md5URL(), srv.dumpURL())
	if err != nil {
		t.Fatalf("ensureLatest returned error: %v", err)
	}
	if !updated {
		t.Error("updated = false, want true (sidecar did not match remote md5)")
	}
	if *srv.dumpHits != 1 {
		t.Errorf("dump endpoint hit %d times, want 1", *srv.dumpHits)
	}
	content, err := os.ReadFile(dbPath)
	if err != nil || string(content) == "stale content" {
		t.Error("dbPath was not replaced with the new dump")
	}
}

func TestEnsureLatest_RejectsCorruptDownload(t *testing.T) {
	gz, md5Hex := buildFixtureDump(t)
	// Corrupt the served bytes without updating the advertised md5, so the
	// download's computed md5 won't match what the md5sum endpoint promised.
	corrupted := append([]byte(nil), gz...)
	corrupted[0] ^= 0xFF

	mux := http.NewServeMux()
	mux.HandleFunc("/md5sum", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  /path\n", md5Hex)
	})
	mux.HandleFunc("/dump", func(w http.ResponseWriter, r *http.Request) {
		w.Write(corrupted)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dbPath := filepath.Join(t.TempDir(), "eve_sde.db")

	updated, err := ensureLatest(dbPath, srv.URL+"/md5sum", srv.URL+"/dump")
	if err == nil {
		t.Fatal("expected an error for a download whose md5 doesn't match, got nil")
	}
	if updated {
		t.Error("updated = true, want false")
	}
	if _, statErr := os.Stat(dbPath); statErr == nil {
		t.Error("dbPath was created despite the download failing its integrity check")
	}
}

func TestEnsureLatest_RejectsUnparseableDump(t *testing.T) {
	// Valid gzip, but the decompressed content isn't a SQLite database at
	// all, so sde.Load's sanity check must catch it.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte("not a sqlite database"))
	gw.Close()
	sum := md5.Sum(buf.Bytes())
	md5Hex := hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/md5sum", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  /path\n", md5Hex)
	})
	mux.HandleFunc("/dump", func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf.Bytes())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dbPath := filepath.Join(t.TempDir(), "eve_sde.db")

	updated, err := ensureLatest(dbPath, srv.URL+"/md5sum", srv.URL+"/dump")
	if err == nil {
		t.Fatal("expected an error for a dump that isn't a valid SDE database, got nil")
	}
	if updated {
		t.Error("updated = true, want false")
	}
	if _, statErr := os.Stat(dbPath); statErr == nil {
		t.Error("dbPath was created despite the dump failing its sanity-load check")
	}
}
