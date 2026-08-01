# TecksDex

A Planetary Industry (PI) profitability calculator for EVE Online. Pick a
target material, a tier to buy raw materials at, and a region, and it works
out the full cost/value/profit rollup layer by layer, a shopping list of
what to buy, and how many factories, links, and Command Centers you'd need
to run the chain continuously.

## Features

- **Profitability rollup** - layer-by-layer cost, value, profit, and margin
  from your chosen buy tier up to the target material, using live market
  prices.
- **Shopping list** - quantities, unit/total cost, cargo volume, and a
  supply chain risk flag (heavily demanded + lightly supplied, or literally
  no sell orders at all) for every raw material you'd need to buy.
- **Factory planning** - how many factories of each tier are needed to keep
  a chosen number of target-tier factories continuously supplied, derived
  from each schematic's real cycle time and output quantity (not a
  hardcoded ratio), including the 3 recipes that skip a tier.
- **Power/CPU and Command Center planning** - power and CPU load per
  factory, the minimum number of links needed to connect everything
  (including the Command Center or Launchpad hub), and the minimum number
  of Command Centers (at a chosen Command Center Upgrades skill level)
  needed to cover it all.
- **Live regional pricing** - Jita, Amarr, Dodixie, and Rens, pulled from
  Fuzzwork's market aggregates API and cached per region.
- **Self-updating game data** - the EVE Static Data Export (SDE) is checked
  against Fuzzwork's latest published build at startup and downloaded
  automatically if it's out of date.
- Built with an eye toward the [autism-friendly design
  principles](designing-for-autistic-people-principles.md) distilled from
  Irina Rusakova's research overview: a muted default colour palette, an
  optional vivid palette and adjustable text size, literal labelling, and
  no unnecessary motion or interruptions.

## Requirements

- Go 1.22+
- Internet access (for live market prices and the SDE update check)

## Getting started

```powershell
cd webapp
go run .
```

Then open `http://localhost:8080`. On first run, if `sde/eve_sde.db` isn't
present, it downloads automatically (see [Data](#data) below).

Useful flags:

- `-addr :8080` - listen address
- `-db sde/eve_sde.db` - path to the SDE sqlite database
- `-skip-sde-update` - skip the startup check against Fuzzwork's latest SDE build

## Testing

```powershell
cd webapp
go test ./...                          # fast, hermetic - no network required
go test -tags=integration ./...        # also exercises the real Fuzzwork APIs
```

## Packaging

`package-release.ps1` (run from the repo root) builds the binary, scans it
for known vulnerabilities with `govulncheck`, and zips it up with the SDE
database into a single folder someone else can unzip and run:

```powershell
.\package-release.ps1
```

## Releases

Pushing a tag like `v1.0.0` triggers [`.github/workflows/release.yml`](.github/workflows/release.yml),
which runs the tests and `govulncheck`, cross-compiles binaries for
Windows, Linux, and macOS (amd64 + arm64), and publishes them to a GitHub
Release. Unlike `package-release.ps1`, these don't bundle the SDE database -
the binary downloads and verifies its own copy on first run, so the
release artifacts are just the executable plus this README.

```bash
git tag v1.0.0
git push origin v1.0.0
```

`workflow_dispatch` (a manual run from the Actions tab) runs the build and
test steps without publishing a release, useful for checking the workflow
itself still passes.

## Data

- The SDE (item data, schematics, facility/Command Center/link stats) comes
  from [Fuzzwork's SDE dump](https://www.fuzzwork.co.uk/dump/) and is
  loaded from `sde/eve_sde.db`. That file isn't checked into this repo
  (it's 400MB+ and changes weekly) - it's downloaded automatically at
  startup instead.
- Live prices come from [Fuzzwork's market aggregates
  API](https://market.fuzzwork.co.uk/api/).

See [`data_locations.txt`](data_locations.txt) for the source links.

## Project layout

```
webapp/
  main.go                    # wiring, flags, embeds web/ via go:embed
  internal/sde/               # loads the SDE into an in-memory Catalog
  internal/sdeupdate/         # checks/downloads a newer SDE build at startup
  internal/market/            # Fuzzwork market price client + cache
  internal/profit/            # cost/value rollup and factory/link/CC planning
  internal/api/               # HTTP JSON handlers
  web/                        # vanilla HTML/CSS/JS frontend
sde/                          # the SDE database lives here (gitignored)
package-release.ps1           # builds + tests + packages a shareable zip
```
