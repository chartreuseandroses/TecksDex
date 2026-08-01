package profit

import (
	"fmt"
	"math"
	"sort"

	"teckdex/webapp/internal/sde"
)

// FactoryRequirement is how many factories producing one specific material
// are needed to keep up with the continuous demand flowing down from the
// factories above it in the chain.
type FactoryRequirement struct {
	TypeID int
	Name   string
	Tier   sde.Tier
	Label  string
	// FacilityType is the PI facility that produces this tier: Basic
	// Industry Facility (P1), Advanced Industry Facility (P2/P3), or
	// High-Tech Production Plant (P4).
	FacilityType string
	// RateNeededPerHour is the aggregate consumption rate demanded of this
	// material by everything above it in the recipe tree - for the target
	// material itself (the top row), this is its own total output rate at
	// targetFactories, since nothing above it in this rollup demands it.
	RateNeededPerHour float64
	// ProductionRatePerHour is what a single factory producing this
	// material yields per hour (its own schematic's OutputQty / CycleTime).
	ProductionRatePerHour float64
	FactoriesNeeded       int
	// PowerLoad/CPULoad are per single factory of this type; TotalPowerLoad/
	// TotalCPULoad are those times FactoriesNeeded - what this row actually
	// costs a planet's Command Center powergrid/CPU budget.
	PowerLoad      float64
	CPULoad        float64
	TotalPowerLoad float64
	TotalCPULoad   float64
}

type FactoryPlan struct {
	TargetFactories int
	// BuildTimeMinutes is how long targetFactories factories, running in
	// parallel, take to produce batchQty units of the target: the number
	// of cycles needed (rounded up - a factory can't run a partial cycle)
	// times the target's own cycle time.
	BuildTimeMinutes int
	Requirements     []FactoryRequirement
	// TotalPowerLoad/TotalCPULoad sum every row's TotalPowerLoad/TotalCPULoad -
	// the combined powergrid/CPU cost of the factories alone, before links.
	TotalPowerLoad float64
	TotalCPULoad   float64
	// TotalFactories is every factory in Requirements (target + all lower
	// tiers), the "buildings" that must each be linked to at least one other.
	TotalFactories int
	// MinLinks is the fewest links that connect every factory AND a hub
	// (a Command Center or Launchpad - materials have to reach one of
	// those to get on/off the planet) into one network: a spanning tree
	// over (TotalFactories + 1) nodes needs that many links.
	MinLinks int
	// LinkPowerLoad/LinkCPULoad are the base cost of a single link;
	// TotalLinkPowerLoad/TotalLinkCPULoad are those times MinLinks.
	LinkPowerLoad      float64
	LinkCPULoad        float64
	TotalLinkPowerLoad float64
	TotalLinkCPULoad   float64
	// CommandCenterSkillLevel is the Command Center Upgrades level (1-5)
	// used to compute MinCommandCenters below.
	CommandCenterSkillLevel  int
	CommandCenterPowerOutput float64
	CommandCenterCPUOutput   float64
	// MinCommandCenters is the fewest Command Centers (at CommandCenterSkillLevel)
	// whose combined power/CPU output covers the factories' load plus the
	// links needed to connect them - whichever of power or CPU is the
	// tighter constraint decides the number.
	MinCommandCenters int
}

// ComputeFactoryPlan works out how many factories of each lower tier are
// needed to keep targetFactories worth of the target material's production
// continuously supplied, all the way down to buyTier (materials at or below
// buyTier are bought, not produced, so they need no factories).
//
// It reuses expandTree's recipe-tree walk (the same one ComputeRollup uses
// for cost quantities) but converts "quantity needed per unit of target"
// into "factories needed" via each schematic's own real cycle time and
// output quantity - which is why this needs no special-casing for the 3
// recipes that skip a tier (a lower-tier material feeding a much higher
// one directly): the rate math works out the same regardless of how many
// tiers apart the two materials are.
func ComputeFactoryPlan(cat *sde.Catalog, target int, buyTier sde.Tier, targetFactories int, batchQty float64, ccSkillLevel int) (*FactoryPlan, error) {
	targetItem, ok := cat.Items[target]
	if !ok {
		return nil, fmt.Errorf("unknown typeID %d", target)
	}
	if buyTier >= targetItem.Tier {
		return nil, fmt.Errorf("buy tier %s must be below target tier %s", buyTier, targetItem.Tier)
	}
	if targetFactories < 1 {
		return nil, fmt.Errorf("factories must be at least 1")
	}
	if ccSkillLevel < 1 || ccSkillLevel > 5 {
		return nil, fmt.Errorf("command center skill level must be 1-5 (a level 0 Command Center can't support PI production)")
	}
	ccCapacity, ok := cat.CommandCenterCapacityByLevel[ccSkillLevel]
	if !ok {
		return nil, fmt.Errorf("no command center capacity data for skill level %d", ccSkillLevel)
	}

	tierAcc := make(map[sde.Tier]map[int]float64)
	expandTree(cat, target, 1, buyTier, tierAcc)

	targetSchem := cat.SchematicForOutput[target]
	targetRatePerHour := float64(targetFactories) * productionRatePerHour(targetSchem)

	unitsPerCycle := float64(targetFactories) * float64(targetSchem.OutputQty)
	cyclesNeeded := math.Ceil(batchQty / unitsPerCycle)
	buildTimeMinutes := int(cyclesNeeded) * (targetSchem.CycleTime / 60)

	plan := &FactoryPlan{TargetFactories: targetFactories, BuildTimeMinutes: buildTimeMinutes}

	targetFacility := facilityTypeForTier(targetItem.Tier)
	targetPower, targetCPU := cat.FacilityStatsByType[targetFacility].PowerLoad, cat.FacilityStatsByType[targetFacility].CPULoad

	// The target material itself is included in Requirements too: it has
	// the highest tier of anything here, so the ascending sort below
	// naturally places it last, mirroring the layer table's last row.
	plan.Requirements = append(plan.Requirements, FactoryRequirement{
		TypeID:                target,
		Name:                  targetItem.Name,
		Tier:                  targetItem.Tier,
		Label:                 targetItem.Tier.String(),
		FacilityType:          targetFacility,
		RateNeededPerHour:     targetRatePerHour,
		ProductionRatePerHour: productionRatePerHour(targetSchem),
		FactoriesNeeded:       targetFactories,
		PowerLoad:             targetPower,
		CPULoad:               targetCPU,
		TotalPowerLoad:        float64(targetFactories) * targetPower,
		TotalCPULoad:          float64(targetFactories) * targetCPU,
	})

	for tier := targetItem.Tier - 1; tier > buyTier; tier-- {
		for typeID, qtyPerUnit := range tierAcc[tier] {
			item := cat.Items[typeID]
			schem := cat.SchematicForOutput[typeID]

			rateNeeded := targetRatePerHour * qtyPerUnit
			rateProduced := productionRatePerHour(schem)
			factoriesNeeded := int(math.Ceil(rateNeeded / rateProduced))

			facilityType := facilityTypeForTier(tier)
			power, cpu := cat.FacilityStatsByType[facilityType].PowerLoad, cat.FacilityStatsByType[facilityType].CPULoad

			plan.Requirements = append(plan.Requirements, FactoryRequirement{
				TypeID:                typeID,
				Name:                  item.Name,
				Tier:                  tier,
				Label:                 tier.String(),
				FacilityType:          facilityType,
				RateNeededPerHour:     rateNeeded,
				ProductionRatePerHour: rateProduced,
				FactoriesNeeded:       factoriesNeeded,
				PowerLoad:             power,
				CPULoad:               cpu,
				TotalPowerLoad:        float64(factoriesNeeded) * power,
				TotalCPULoad:          float64(factoriesNeeded) * cpu,
			})
		}
	}

	sort.SliceStable(plan.Requirements, func(i, j int) bool {
		a, b := plan.Requirements[i], plan.Requirements[j]
		if a.Tier != b.Tier {
			return a.Tier < b.Tier // lowest tier first, target last - matches the layer table's flow
		}
		return a.Name < b.Name
	})

	for _, req := range plan.Requirements {
		plan.TotalPowerLoad += req.TotalPowerLoad
		plan.TotalCPULoad += req.TotalCPULoad
		plan.TotalFactories += req.FactoriesNeeded
	}

	// A spanning tree over the factories plus one hub node (Command Center
	// or Launchpad) needs (TotalFactories + 1 - 1) = TotalFactories links.
	// Even a single factory needs one link, to reach that hub.
	plan.MinLinks = plan.TotalFactories
	plan.LinkPowerLoad = cat.LinkStats.PowerLoad
	plan.LinkCPULoad = cat.LinkStats.CPULoad
	plan.TotalLinkPowerLoad = float64(plan.MinLinks) * plan.LinkPowerLoad
	plan.TotalLinkCPULoad = float64(plan.MinLinks) * plan.LinkCPULoad

	plan.CommandCenterSkillLevel = ccSkillLevel
	plan.CommandCenterPowerOutput = ccCapacity.PowerOutput
	plan.CommandCenterCPUOutput = ccCapacity.CPUOutput

	combinedPower := plan.TotalPowerLoad + plan.TotalLinkPowerLoad
	combinedCPU := plan.TotalCPULoad + plan.TotalLinkCPULoad
	minByPower := int(math.Ceil(combinedPower / ccCapacity.PowerOutput))
	minByCPU := int(math.Ceil(combinedCPU / ccCapacity.CPUOutput))
	plan.MinCommandCenters = max(1, minByPower, minByCPU)

	return plan, nil
}

func productionRatePerHour(schem sde.Schematic) float64 {
	return float64(schem.OutputQty) / (float64(schem.CycleTime) / 3600.0)
}

func facilityTypeForTier(tier sde.Tier) string {
	switch tier {
	case sde.TierP1:
		return "Basic Industry Facility"
	case sde.TierP2, sde.TierP3:
		return "Advanced Industry Facility"
	case sde.TierP4:
		return "High-Tech Production Plant"
	default:
		return ""
	}
}
