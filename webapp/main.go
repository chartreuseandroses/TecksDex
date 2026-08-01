// Command webapp serves a PI profitability calculator backed by the EVE
// SDE and live Fuzzwork market prices. Run from the repo root with
// `go run ./webapp` so the default -db path resolves.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"

	"teckdex/webapp/internal/api"
	"teckdex/webapp/internal/market"
	"teckdex/webapp/internal/sde"
	"teckdex/webapp/internal/sdeupdate"
)

//go:embed web
var webAssets embed.FS

func main() {
	dbPath := flag.String("db", "sde/eve_sde.db", "path to the EVE SDE sqlite database")
	addr := flag.String("addr", ":8080", "listen address")
	skipSDEUpdate := flag.Bool("skip-sde-update", false, "don't check Fuzzwork for a newer SDE dump at startup")
	flag.Parse()

	if !*skipSDEUpdate {
		// A failed check is never fatal - worst case we keep using whatever
		// SDE is already on disk, which is exactly what happens if this
		// were skipped entirely.
		if updated, err := sdeupdate.EnsureLatest(*dbPath); err != nil {
			log.Printf("SDE update check failed, continuing with existing data: %v", err)
		} else if updated {
			log.Printf("downloaded a newer SDE dump to %s", *dbPath)
		} else {
			log.Printf("SDE dump is already up to date")
		}
	}

	cat, err := sde.Load(*dbPath)
	if err != nil {
		log.Fatalf("load SDE: %v", err)
	}
	log.Printf("loaded %d PI items, %d schematics", len(cat.Items), len(cat.SchematicForOutput))

	srv := api.NewServer(cat, market.NewClient())

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	staticFS, err := fs.Sub(webAssets, "web")
	if err != nil {
		log.Fatalf("mount embedded web assets: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
