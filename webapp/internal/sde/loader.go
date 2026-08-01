// Package sde loads the EVE Static Data Export subset needed for Planetary
// Industry profitability calculations into an in-memory Catalog.
package sde

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Tier is a Planetary Industry material tier, P0 (raw) through P4 (advanced).
type Tier int

const (
	TierP0 Tier = 0
	TierP1 Tier = 1
	TierP2 Tier = 2
	TierP3 Tier = 3
	TierP4 Tier = 4
)

func (t Tier) String() string {
	names := [...]string{"P0", "P1", "P2", "P3", "P4"}
	if t < 0 || int(t) >= len(names) {
		return "?"
	}
	return names[t]
}

// marketGroupTier maps invTypes.marketGroupID to its PI tier.
var marketGroupTier = map[int]Tier{
	1333: TierP0,
	1334: TierP1,
	1335: TierP2,
	1336: TierP3,
	1337: TierP4,
}

type Item struct {
	TypeID      int
	Name        string
	Description string
	Tier        Tier
	// Volume is the per-unit cargo volume in m3 (invTypes.volume).
	Volume float64
}

type SchematicInput struct {
	TypeID   int
	Quantity int
}

type Schematic struct {
	ID           int
	Name         string
	CycleTime    int
	OutputTypeID int
	OutputQty    int
	Inputs       []SchematicInput
}

// FacilityStats is the powergrid/CPU cost of running one instance of a PI
// production facility, read from dgmTypeAttributes (attributeID 15 =
// powerLoad, 49 = cpuLoad). There are 3 facility tiers - Basic Industry
// Facility (P1), Advanced Industry Facility (P2/P3), High-Tech Production
// Plant (P4) - each deployed as 8 separate typeIDs (one per planet type);
// Load verifies all 8 variants of a tier agree rather than trusting one.
type FacilityStats struct {
	PowerLoad float64
	CPULoad   float64
}

// CommandCenterCapacity is the powergrid/CPU a Command Center provides at a
// given Command Center Upgrades skill level, read from dgmTypeAttributes
// (attributeID 11 = powerOutput, 48 = cpuOutput). The skill has levels 0-5;
// each level's output is pre-baked into its own (unpublished - not something
// you deploy separately, just reference data for what your one deployed
// Command Center scales to as you train the skill) typeID in the SDE.
type CommandCenterCapacity struct {
	PowerOutput float64
	CPUOutput   float64
}

// Catalog is the full in-memory PI dataset: every material item across all
// five tiers, the schematic that produces each non-P0 item, the
// powergrid/CPU cost of each facility tier (keyed by facility type name:
// "Basic Industry Facility", "Advanced Industry Facility", "High-Tech
// Production Plant"), Command Center capacity by skill level (0-5), and the
// base powergrid/CPU cost of a single link (invTypes.groupID 1036's one
// "Link" item) - its per-km and per-level scaling factors are ignored since
// this app has no notion of planet layout/distance to apply them against.
type Catalog struct {
	Items                        map[int]Item
	ItemsByTier                  map[Tier][]Item
	SchematicForOutput           map[int]Schematic
	FacilityStatsByType          map[string]FacilityStats
	CommandCenterCapacityByLevel map[int]CommandCenterCapacity
	LinkStats                    FacilityStats
}

// Load opens the SDE sqlite database at dbPath and builds a Catalog.
func Load(dbPath string) (*Catalog, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sde db: %w", err)
	}
	defer db.Close()

	cat := &Catalog{
		Items:              make(map[int]Item),
		ItemsByTier:        make(map[Tier][]Item),
		SchematicForOutput: make(map[int]Schematic),
	}

	facilityStats, err := loadFacilityStats(db)
	if err != nil {
		return nil, err
	}
	cat.FacilityStatsByType = facilityStats

	ccCapacity, err := loadCommandCenterCapacity(db)
	if err != nil {
		return nil, err
	}
	cat.CommandCenterCapacityByLevel = ccCapacity

	linkStats, err := loadLinkStats(db)
	if err != nil {
		return nil, err
	}
	cat.LinkStats = linkStats

	itemRows, err := db.Query(
		`SELECT typeID, typeName, COALESCE(description, ''), marketGroupID, COALESCE(volume, 0)
		 FROM invTypes
		 WHERE marketGroupID IN (1333, 1334, 1335, 1336, 1337)`,
	)
	if err != nil {
		return nil, fmt.Errorf("query invTypes: %w", err)
	}
	defer itemRows.Close()

	for itemRows.Next() {
		var it Item
		var marketGroupID int
		if err := itemRows.Scan(&it.TypeID, &it.Name, &it.Description, &marketGroupID, &it.Volume); err != nil {
			return nil, fmt.Errorf("scan invTypes row: %w", err)
		}
		it.Tier = marketGroupTier[marketGroupID]
		cat.Items[it.TypeID] = it
		cat.ItemsByTier[it.Tier] = append(cat.ItemsByTier[it.Tier], it)
	}
	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invTypes rows: %w", err)
	}

	schemaRows, err := db.Query(`SELECT schematicID, schematicName, cycleTime FROM planetSchematics`)
	if err != nil {
		return nil, fmt.Errorf("query planetSchematics: %w", err)
	}
	defer schemaRows.Close()

	schematics := make(map[int]*Schematic)
	for schemaRows.Next() {
		var s Schematic
		if err := schemaRows.Scan(&s.ID, &s.Name, &s.CycleTime); err != nil {
			return nil, fmt.Errorf("scan planetSchematics row: %w", err)
		}
		schematics[s.ID] = &s
	}
	if err := schemaRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planetSchematics rows: %w", err)
	}

	mapRows, err := db.Query(
		`SELECT schematicID, typeID, quantity, isInput FROM planetSchematicsTypeMap`,
	)
	if err != nil {
		return nil, fmt.Errorf("query planetSchematicsTypeMap: %w", err)
	}
	defer mapRows.Close()

	for mapRows.Next() {
		var schematicID, typeID, quantity int
		var isInput bool
		if err := mapRows.Scan(&schematicID, &typeID, &quantity, &isInput); err != nil {
			return nil, fmt.Errorf("scan planetSchematicsTypeMap row: %w", err)
		}
		s, ok := schematics[schematicID]
		if !ok {
			return nil, fmt.Errorf("planetSchematicsTypeMap references unknown schematicID %d", schematicID)
		}
		if isInput {
			s.Inputs = append(s.Inputs, SchematicInput{TypeID: typeID, Quantity: quantity})
		} else {
			s.OutputTypeID = typeID
			s.OutputQty = quantity
		}
	}
	if err := mapRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planetSchematicsTypeMap rows: %w", err)
	}

	for _, s := range schematics {
		if s.OutputTypeID == 0 {
			return nil, fmt.Errorf("schematic %d (%s) has no output row", s.ID, s.Name)
		}
		cat.SchematicForOutput[s.OutputTypeID] = *s
	}

	return cat, nil
}

const (
	industryFacilityGroupID = 1028
	powerLoadAttributeID    = 15
	cpuLoadAttributeID      = 49
)

// loadFacilityStats reads the powergrid/CPU cost of each industry facility
// tier (invTypes.groupID 1028: Basic/Advanced Industry Facility, High-Tech
// Production Plant), each of which exists as up to 8 separate typeIDs (one
// per planet type). It verifies every variant of a tier reports the same
// cost rather than trusting a single sample.
func loadFacilityStats(db *sql.DB) (map[string]FacilityStats, error) {
	rows, err := db.Query(
		`SELECT it.typeName, a.attributeID, COALESCE(a.valueInt, a.valueFloat)
		 FROM invTypes it
		 JOIN dgmTypeAttributes a ON a.typeID = it.typeID
		 WHERE it.groupID = ? AND a.attributeID IN (?, ?)`,
		industryFacilityGroupID, powerLoadAttributeID, cpuLoadAttributeID,
	)
	if err != nil {
		return nil, fmt.Errorf("query facility power/cpu attributes: %w", err)
	}
	defer rows.Close()

	powerSamples := make(map[string][]float64)
	cpuSamples := make(map[string][]float64)
	for rows.Next() {
		var typeName string
		var attributeID int
		var value float64
		if err := rows.Scan(&typeName, &attributeID, &value); err != nil {
			return nil, fmt.Errorf("scan facility attribute row: %w", err)
		}
		facilityType, ok := canonicalFacilityType(typeName)
		if !ok {
			continue
		}
		switch attributeID {
		case powerLoadAttributeID:
			powerSamples[facilityType] = append(powerSamples[facilityType], value)
		case cpuLoadAttributeID:
			cpuSamples[facilityType] = append(cpuSamples[facilityType], value)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate facility attribute rows: %w", err)
	}

	stats := make(map[string]FacilityStats)
	for facilityType, values := range powerSamples {
		power, err := allEqual(values)
		if err != nil {
			return nil, fmt.Errorf("%s powerLoad: %w", facilityType, err)
		}
		cpu, err := allEqual(cpuSamples[facilityType])
		if err != nil {
			return nil, fmt.Errorf("%s cpuLoad: %w", facilityType, err)
		}
		stats[facilityType] = FacilityStats{PowerLoad: power, CPULoad: cpu}
	}
	return stats, nil
}

func canonicalFacilityType(typeName string) (string, bool) {
	switch {
	case strings.Contains(typeName, "High-Tech Production Plant"):
		return "High-Tech Production Plant", true
	case strings.Contains(typeName, "Advanced Industry Facility"):
		return "Advanced Industry Facility", true
	case strings.Contains(typeName, "Basic Industry Facility"):
		return "Basic Industry Facility", true
	default:
		return "", false
	}
}

func allEqual(values []float64) (float64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("no values found")
	}
	for _, v := range values[1:] {
		if v != values[0] {
			return 0, fmt.Errorf("inconsistent values across facility variants: %v", values)
		}
	}
	return values[0], nil
}

const (
	commandCenterGroupID   = 1027
	powerOutputAttributeID = 11
	cpuOutputAttributeID   = 48
)

// loadCommandCenterCapacity reads Command Center powergrid/CPU output at
// each Command Center Upgrades skill level (0-5), from the 8 planet-type
// variants per level (invTypes.groupID 1027). It verifies all 8 variants of
// a level agree rather than trusting a single sample.
func loadCommandCenterCapacity(db *sql.DB) (map[int]CommandCenterCapacity, error) {
	rows, err := db.Query(
		`SELECT it.typeName, a.attributeID, COALESCE(a.valueInt, a.valueFloat)
		 FROM invTypes it
		 JOIN dgmTypeAttributes a ON a.typeID = it.typeID
		 WHERE it.groupID = ? AND a.attributeID IN (?, ?)`,
		commandCenterGroupID, powerOutputAttributeID, cpuOutputAttributeID,
	)
	if err != nil {
		return nil, fmt.Errorf("query command center power/cpu attributes: %w", err)
	}
	defer rows.Close()

	powerSamples := make(map[int][]float64)
	cpuSamples := make(map[int][]float64)
	for rows.Next() {
		var typeName string
		var attributeID int
		var value float64
		if err := rows.Scan(&typeName, &attributeID, &value); err != nil {
			return nil, fmt.Errorf("scan command center attribute row: %w", err)
		}
		level, ok := commandCenterSkillLevel(typeName)
		if !ok {
			continue
		}
		switch attributeID {
		case powerOutputAttributeID:
			powerSamples[level] = append(powerSamples[level], value)
		case cpuOutputAttributeID:
			cpuSamples[level] = append(cpuSamples[level], value)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate command center attribute rows: %w", err)
	}

	capacity := make(map[int]CommandCenterCapacity)
	for level, values := range powerSamples {
		power, err := allEqual(values)
		if err != nil {
			return nil, fmt.Errorf("command center level %d powerOutput: %w", level, err)
		}
		cpu, err := allEqual(cpuSamples[level])
		if err != nil {
			return nil, fmt.Errorf("command center level %d cpuOutput: %w", level, err)
		}
		capacity[level] = CommandCenterCapacity{PowerOutput: power, CPUOutput: cpu}
	}
	return capacity, nil
}

// commandCenterSkillLevel maps a Command Center typeName to the Command
// Center Upgrades skill level whose output it represents: the plain
// "[Planet Type] Command Center" is level 0, and Limited/Standard/Improved/
// Advanced/Elite are levels 1-5.
func commandCenterSkillLevel(typeName string) (int, bool) {
	switch {
	case strings.HasPrefix(typeName, "Elite "):
		return 5, true
	case strings.HasPrefix(typeName, "Advanced "):
		return 4, true
	case strings.HasPrefix(typeName, "Improved "):
		return 3, true
	case strings.HasPrefix(typeName, "Standard "):
		return 2, true
	case strings.HasPrefix(typeName, "Limited "):
		return 1, true
	case strings.HasSuffix(typeName, "Command Center"):
		return 0, true
	default:
		return -1, false
	}
}

const linkGroupID = 1036

// loadLinkStats reads the base powergrid/CPU cost of a single link
// (invTypes.groupID 1036, the one "Link" item). This ignores the per-km and
// per-level scaling attributes also present on that item, since nothing in
// this app models planet layout or distance between pins.
func loadLinkStats(db *sql.DB) (FacilityStats, error) {
	rows, err := db.Query(
		`SELECT a.attributeID, COALESCE(a.valueInt, a.valueFloat)
		 FROM invTypes it
		 JOIN dgmTypeAttributes a ON a.typeID = it.typeID
		 WHERE it.groupID = ? AND a.attributeID IN (?, ?)`,
		linkGroupID, powerLoadAttributeID, cpuLoadAttributeID,
	)
	if err != nil {
		return FacilityStats{}, fmt.Errorf("query link power/cpu attributes: %w", err)
	}
	defer rows.Close()

	var stats FacilityStats
	var havePower, haveCPU bool
	for rows.Next() {
		var attributeID int
		var value float64
		if err := rows.Scan(&attributeID, &value); err != nil {
			return FacilityStats{}, fmt.Errorf("scan link attribute row: %w", err)
		}
		switch attributeID {
		case powerLoadAttributeID:
			stats.PowerLoad = value
			havePower = true
		case cpuLoadAttributeID:
			stats.CPULoad = value
			haveCPU = true
		}
	}
	if err := rows.Err(); err != nil {
		return FacilityStats{}, fmt.Errorf("iterate link attribute rows: %w", err)
	}
	if !havePower || !haveCPU {
		return FacilityStats{}, fmt.Errorf("link powerLoad/cpuLoad attributes not found")
	}
	return stats, nil
}
