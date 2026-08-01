const regionSelect = document.getElementById('region');
const targetSelect = document.getElementById('target');
const buyTierSelect = document.getElementById('buyTier');
const qtyInput = document.getElementById('qty');
const factoriesInput = document.getElementById('factories');
const ccSkillLevelSelect = document.getElementById('ccSkillLevel');
const form = document.getElementById('calc-form');

const warningsEl = document.getElementById('warnings');
const summaryEl = document.getElementById('summary');
const layersSection = document.getElementById('layers-section');
const layersBody = document.querySelector('#layers-table tbody');
const shoppingSection = document.getElementById('shopping-section');
const shoppingBody = document.querySelector('#shopping-table tbody');
const shoppingFoot = document.querySelector('#shopping-table tfoot');
const factoriesSection = document.getElementById('factories-section');
const factoriesBody = document.querySelector('#factories-table tbody');
const colorIntensitySelect = document.getElementById('colorIntensity');
const textSizeSelect = document.getElementById('textSize');

let tiers = [];

// Personalisation: remembers the user's choice of colour intensity and text
// size across visits rather than forcing one fixed default on everyone.
const DISPLAY_PREFS_KEY = 'pi-display-prefs';

function loadDisplayPrefs() {
  let prefs = {};
  try {
    prefs = JSON.parse(localStorage.getItem(DISPLAY_PREFS_KEY)) || {};
  } catch (e) {
    prefs = {};
  }
  const colorIntensity = prefs.colorIntensity === 'vivid' ? 'vivid' : 'calm';
  const textSize = ['small', 'large'].includes(prefs.textSize) ? prefs.textSize : 'normal';

  document.documentElement.dataset.colorIntensity = colorIntensity;
  document.documentElement.dataset.textSize = textSize;
  colorIntensitySelect.value = colorIntensity;
  textSizeSelect.value = textSize;
}

function saveDisplayPrefs() {
  const prefs = {
    colorIntensity: colorIntensitySelect.value,
    textSize: textSizeSelect.value,
  };
  document.documentElement.dataset.colorIntensity = prefs.colorIntensity;
  document.documentElement.dataset.textSize = prefs.textSize;
  try {
    localStorage.setItem(DISPLAY_PREFS_KEY, JSON.stringify(prefs));
  } catch (e) {
    // Personalisation is a nicety, not a requirement - ignore storage failures.
  }
}

loadDisplayPrefs();
colorIntensitySelect.addEventListener('change', saveDisplayPrefs);
textSizeSelect.addEventListener('change', saveDisplayPrefs);

async function fetchJSON(url) {
  const resp = await fetch(url);
  const body = await resp.json();
  if (!resp.ok) {
    throw new Error(body.error || `request to ${url} failed`);
  }
  return body;
}

function populateRegions(regions) {
  regionSelect.innerHTML = '';
  for (const r of regions) {
    const opt = document.createElement('option');
    opt.value = r.RegionID;
    opt.textContent = r.Name;
    regionSelect.appendChild(opt);
  }
}

function populateTargets() {
  const previousValue = targetSelect.value;

  targetSelect.innerHTML = '';
  // P0 has no recipe, so it can never be a target; every other tier can be.
  for (const group of tiers.filter(t => t.tier >= 1)) {
    const optgroup = document.createElement('optgroup');
    optgroup.label = group.label;
    for (const item of group.items || []) {
      const opt = document.createElement('option');
      opt.value = item.typeId;
      opt.dataset.tier = group.tier;
      const tags = [item.demand, item.supply].filter(Boolean).join(', ');
      opt.textContent = tags ? `${item.name} (${tags})` : item.name;
      optgroup.appendChild(opt);
    }
    targetSelect.appendChild(optgroup);
  }

  if (previousValue && targetSelect.querySelector(`option[value="${previousValue}"]`)) {
    targetSelect.value = previousValue;
  } else {
    // Default to the first P4 item, per the plan.
    const p4First = targetSelect.querySelector('option[data-tier="4"]');
    if (p4First) targetSelect.value = p4First.value;
  }
}

async function loadTiersForRegion(regionId) {
  tiers = await fetchJSON(`/api/tiers?region=${regionId}`);
  populateTargets();
  updateBuyTierOptions();
}

function updateBuyTierOptions() {
  const selectedOption = targetSelect.selectedOptions[0];
  const targetTier = selectedOption ? Number(selectedOption.dataset.tier) : 4;

  buyTierSelect.innerHTML = '';
  for (const group of tiers.filter(t => t.tier < targetTier)) {
    const opt = document.createElement('option');
    opt.value = group.tier;
    opt.textContent = group.label;
    buyTierSelect.appendChild(opt);
  }

  // Default to P1 where available; falls back to the lowest tier below the
  // target (P0) when the target itself is P1, since P1 isn't an option then.
  const p1 = buyTierSelect.querySelector('option[value="1"]');
  buyTierSelect.value = p1 ? p1.value : buyTierSelect.options[0]?.value;
}

function money(n) {
  return n.toLocaleString(undefined, { maximumFractionDigits: 2 }) + ' ISK';
}

function profitClass(n) {
  return n >= 0 ? 'profit-positive' : 'profit-negative';
}

function marginPercent(profit, cost) {
  return cost > 0 ? (profit / cost) * 100 : 0;
}

function volume(n) {
  return n.toLocaleString(undefined, { maximumFractionDigits: 2 }) + ' m3';
}

function rate(n) {
  return n.toLocaleString(undefined, { maximumFractionDigits: 2 });
}

function formatMinutes(totalMinutes) {
  if (totalMinutes < 60) return `${totalMinutes} min`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return minutes === 0 ? `${hours}h` : `${hours}h ${minutes}m`;
}

function load(n) {
  return n.toLocaleString(undefined, { maximumFractionDigits: 2 });
}

function renderResult(res) {
  // "(buy)" warnings are about shopping list items specifically - those are
  // now shown inline in the Supply Chain Risk column instead, so surfacing
  // them again here would just be the same information twice.
  const bannerWarnings = (res.Warnings || []).filter(w => !w.endsWith('(buy)'));
  if (bannerWarnings.length > 0) {
    warningsEl.hidden = false;
    warningsEl.innerHTML = '<strong>Warnings:</strong><ul>' +
      bannerWarnings.map(w => `<li>${w}</li>`).join('') + '</ul>';
  } else {
    warningsEl.hidden = true;
    warningsEl.innerHTML = '';
  }

  summaryEl.hidden = false;
  document.getElementById('sum-cost').textContent = money(res.TotalCost);
  document.getElementById('sum-value').textContent = money(res.TotalValue);
  const profitCell = document.getElementById('sum-profit');
  profitCell.textContent = money(res.TotalProfit);
  profitCell.className = profitClass(res.TotalProfit);
  document.getElementById('sum-margin').textContent = marginPercent(res.TotalProfit, res.TotalCost).toFixed(1) + '%';
  document.getElementById('sum-build-time').textContent =
    res.FactoryPlan ? formatMinutes(res.FactoryPlan.BuildTimeMinutes) : '—';

  document.getElementById('sum-power-load').textContent =
    res.FactoryPlan ? load(res.FactoryPlan.TotalPowerLoad) : '—';
  document.getElementById('sum-cpu-load').textContent =
    res.FactoryPlan ? load(res.FactoryPlan.TotalCPULoad) : '—';
  document.getElementById('sum-min-links').textContent =
    res.FactoryPlan ? res.FactoryPlan.MinLinks : '—';
  document.getElementById('sum-min-cc').textContent =
    res.FactoryPlan ? `${res.FactoryPlan.MinCommandCenters} (at Command Center Upgrades level ${res.FactoryPlan.CommandCenterSkillLevel})` : '—';

  layersSection.hidden = false;
  layersBody.innerHTML = '';
  for (const layer of res.Layers || []) {
    const itemsStr = (layer.Items || [])
      .map(it => `${it.Name} x${it.Quantity.toLocaleString(undefined, { maximumFractionDigits: 2 })}`)
      .join(', ');
    const margin = marginPercent(layer.Profit, layer.Cost);
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${layer.Label}</td>
      <td>${money(layer.Cost)}</td>
      <td>${money(layer.Value)}</td>
      <td class="${profitClass(layer.Profit)}">${money(layer.Profit)}</td>
      <td class="${profitClass(margin)}">${margin.toFixed(1)}%</td>
      <td>${itemsStr}</td>
    `;
    layersBody.appendChild(tr);
  }

  shoppingSection.hidden = false;
  shoppingBody.innerHTML = '';
  shoppingFoot.innerHTML = '';
  let totalQty = 0, totalCost = 0, totalVolume = 0;
  for (const entry of res.ShoppingList || []) {
    const demandSupply = [entry.Demand, entry.Supply].filter(Boolean).join(', ');
    let riskLabel, isRisk;
    if (!entry.HasSellOrders) {
      // Nothing is instantly buyable here at all - UnitCost/TotalCost are 0
      // because there's no ask price, not because the material is free.
      riskLabel = `No sell orders, buy order required (${entry.Demand || 'demand unknown'})`;
      isRisk = true;
    } else {
      riskLabel = entry.Risk ? `⚠ ${demandSupply}` : (demandSupply || '—');
      isRisk = entry.Risk;
    }
    totalQty += entry.Quantity;
    totalCost += entry.TotalCost;
    totalVolume += entry.TotalVolumeM3;
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${entry.Name}</td>
      <td>${entry.Quantity.toLocaleString(undefined, { maximumFractionDigits: 2 })}</td>
      <td>${money(entry.UnitCost)}</td>
      <td>${money(entry.TotalCost)}</td>
      <td>${volume(entry.UnitVolumeM3)}</td>
      <td>${volume(entry.TotalVolumeM3)}</td>
      <td class="${isRisk ? 'risk-flag' : ''}">${riskLabel}</td>
    `;
    shoppingBody.appendChild(tr);
  }

  if ((res.ShoppingList || []).length > 0) {
    const totalRow = document.createElement('tr');
    totalRow.innerHTML = `
      <th>Total</th>
      <th>${totalQty.toLocaleString(undefined, { maximumFractionDigits: 2 })}</th>
      <th></th>
      <th>${money(totalCost)}</th>
      <th></th>
      <th>${volume(totalVolume)}</th>
      <th></th>
    `;
    shoppingFoot.appendChild(totalRow);
  }

  const requirements = (res.FactoryPlan && res.FactoryPlan.Requirements) || [];
  factoriesSection.hidden = false;
  factoriesBody.innerHTML = '';
  for (const req of requirements) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${req.Label}</td>
      <td>${req.Name}</td>
      <td>${req.FacilityType}</td>
      <td>${rate(req.RateNeededPerHour)}</td>
      <td>${rate(req.ProductionRatePerHour)}</td>
      <td>${req.FactoriesNeeded}</td>
      <td>${load(req.TotalPowerLoad)} (${load(req.PowerLoad)} x${req.FactoriesNeeded})</td>
      <td>${load(req.TotalCPULoad)} (${load(req.CPULoad)} x${req.FactoriesNeeded})</td>
    `;
    factoriesBody.appendChild(tr);
  }
}

async function runCalculation() {
  const params = new URLSearchParams({
    target: targetSelect.value,
    buyTier: buyTierSelect.value,
    qty: qtyInput.value || '1',
    factories: factoriesInput.value || '1',
    ccSkillLevel: ccSkillLevelSelect.value,
    region: regionSelect.value,
  });
  const res = await fetchJSON(`/api/profitability?${params}`);
  renderResult(res);
}

targetSelect.addEventListener('change', updateBuyTierOptions);

regionSelect.addEventListener('change', () => {
  loadTiersForRegion(regionSelect.value).catch(err => {
    warningsEl.hidden = false;
    warningsEl.innerHTML = `<strong>Error:</strong> ${err.message}`;
  });
});

form.addEventListener('submit', (e) => {
  e.preventDefault();
  runCalculation().catch(err => {
    warningsEl.hidden = false;
    warningsEl.innerHTML = `<strong>Error:</strong> ${err.message}`;
  });
});

(async function init() {
  const regions = await fetchJSON('/api/regions');
  populateRegions(regions);
  await loadTiersForRegion(regionSelect.value);
  await runCalculation();
})().catch(err => {
  warningsEl.hidden = false;
  warningsEl.innerHTML = `<strong>Error loading page:</strong> ${err.message}`;
});
