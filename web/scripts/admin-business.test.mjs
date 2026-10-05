// Pure logic of the Business page (projection simulator), checked on the real recorded API responses.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const B = await import('../src/components/admin/pages/business.logic.ts');

const real = (name) => JSON.parse(readFileSync(new URL(`../src/lib/admin/real/${name}.json`, import.meta.url), 'utf8'));
const nbsp = (s) => s.replace(/[  ]/g, ' ');

/** A baseline with every measure present, to test the model itself. */
const BASE = {
  users: 1000, peak: 10, peakEstimated: true, peakTruncated: false,
  serverMonthly: 100, storageMonthly: 20, bandwidthPerHour: 0.01,
  hoursPerMonth: 600, prorata: 1,
};
const FULL = { growth: 10, share: 50, hoursPerActive: 2, gbPerHour: null, costPerGb: null, streamLimit: 20, tariff: 3, conversion: 5 };
const finiteOrNull = (v) => v === null || Number.isFinite(v);

test('safeDiv and mul never produce Infinity, NaN or a fake 0', () => {
  assert.equal(B.safeDiv(1, 0), null);
  assert.equal(B.safeDiv(0, 0), null);
  assert.equal(B.safeDiv(null, 2), null);
  assert.equal(B.safeDiv(2, null), null);
  assert.equal(B.safeDiv(Infinity, 2), null);
  assert.equal(B.safeDiv(0, 5), 0);
  assert.equal(B.safeDiv(6, 3), 2);
  assert.equal(B.mul(null, 2), null);
  assert.equal(B.mul(0, 2), 0);
});

test('baseline from the real costs + overview: server per month = value / prorata, growth scaled to 30 days', () => {
  const costs = real('costs');
  const overview = real('overview');
  const { baseline, defaults, measuredGrowth } = B.buildBaseline(costs.data, overview.data, 7);
  assert.ok(Math.abs(baseline.serverMonthly - 300) < 1e-9, '70 / 0.2333 = 300');
  assert.equal(baseline.storageMonthly, null);
  assert.equal(baseline.users, 4);
  // 4 users, 2 new, 7 days: sample far too small -> no growth proposed (was ~429 % / month)
  assert.equal(measuredGrowth, null);
  assert.equal(defaults.growth, null);
  // no active user measured: hours per actif is unknown (null), not 0 and not Infinity
  assert.equal(defaults.hoursPerActive, null);
  // nothing measurable for these: they start empty
  assert.equal(defaults.streamLimit, null);
  assert.equal(defaults.tariff, null);
  assert.equal(defaults.conversion, null);
  assert.equal(defaults.gbPerHour, null);
  assert.equal(defaults.costPerGb, null);
  // measured bandwidth cost per hour = 0.6 / 3 h
  assert.ok(Math.abs(baseline.bandwidthPerHour - 0.2) < 1e-9);
});

test('baseline on the partial (30 days) and empty responses', () => {
  const partial = B.buildBaseline(real('costs.partial').data, real('overview').data, 30);
  assert.equal(partial.baseline.serverMonthly, 300);
  assert.equal(partial.baseline.bandwidthPerHour, null, 'bandwidth inputs missing');
  const empty = B.buildBaseline(real('costs.empty').data, real('overview.empty').data, 30);
  assert.ok(Object.values(empty.defaults).every((v) => v === null || Number.isFinite(v)));
  assert.ok(Object.values(empty.baseline).every((v) => typeof v === 'boolean' || finiteOrNull(v)));
});

test('baseline: no new users / empty history gives a null growth, not 0 or Infinity', () => {
  const costs = real('costs').data;
  const ov = real('overview').data;
  const same = structuredClone(ov);
  same.kpis.new_users.value = same.kpis.total_users.value; // everyone is new: nothing before
  assert.equal(B.buildBaseline(costs, same, 7).measuredGrowth, null);
  const zero = structuredClone(ov);
  zero.kpis.total_users.value = 0;
  zero.kpis.new_users.value = 0;
  assert.equal(B.buildBaseline(costs, zero, 7).measuredGrowth, null);
  assert.equal(B.buildBaseline(costs, ov, 0).measuredGrowth, null, '0 days');
  assert.equal(B.buildBaseline(costs, ov, 0).baseline.hoursPerMonth, null);
});

test('the curve of registered users: month 0 is today, then compound growth', () => {
  const rows = B.projectScenario(BASE, FULL, 'tendance');
  assert.equal(rows.length, 13);
  assert.equal(rows[0].users, 1000);
  assert.ok(Math.abs(rows[1].users - 1100) < 1e-9);
  assert.ok(Math.abs(rows[12].users - 1000 * Math.pow(1.1, 12)) < 1e-6);
  for (let m = 1; m <= 12; m++) assert.ok(rows[m].users > rows[m - 1].users);
});

test('hours, actives and cost follow the assumptions', () => {
  const rows = B.projectScenario(BASE, FULL, 'tendance');
  assert.equal(rows[0].actives, 500);
  assert.equal(rows[0].hours, 1000);
  // month 0: 1 machine (peak 10 < 20) + measured bandwidth 1000 h x 0.01 + storage 20
  assert.equal(rows[0].servers, 1);
  assert.ok(Math.abs(rows[0].cost - (100 + 10 + 20)) < 1e-9);
  assert.equal(rows[0].costPartial, false);
  assert.ok(Math.abs(rows[0].costPerHour - 0.13) < 1e-9);
  assert.ok(Math.abs(rows[0].costPerActive - 0.26) < 1e-9);
  // peak follows the hours
  assert.ok(Math.abs(rows[12].peak - 10 * (rows[12].hours / rows[0].hours)) < 1e-9);
  assert.ok(rows[12].servers >= 2);
  assert.ok(rows[12].cost > rows[0].cost);
  assert.ok(Math.abs(rows[0].revenue - 500 * 0.05 * 3) < 1e-9);
});

test('typed GB per hour x cost per GB replaces the measured bandwidth cost', () => {
  const a = { ...FULL, gbPerHour: 1, costPerGb: 0.5 };
  assert.equal(B.bandwidthPerHour(BASE, a), 0.5);
  assert.equal(B.bandwidthPerHour(BASE, { ...FULL, gbPerHour: 1, costPerGb: null }), 0.01, 'one of the two is empty: measured one');
  const rows = B.projectScenario(BASE, a, 'tendance');
  assert.ok(Math.abs(rows[0].bandwidthCost - 500) < 1e-9);
});

test('prudent < tendance < ambitieux', () => {
  const all = B.projectAll(BASE, FULL);
  for (const key of ['users', 'actives', 'hours', 'cost', 'revenue']) {
    const p = all.prudent[12][key], t = all.tendance[12][key], a = all.ambitieux[12][key];
    assert.ok(p < t && t < a, `${key}: ${p} < ${t} < ${a}`);
  }
  assert.equal(all.prudent[0].users, all.ambitieux[0].users, 'same start');
  assert.deepEqual(B.SCENARIOS.map((s) => s.mult), [0.5, 1, 1.5]);
});

test('milestones at 3, 6 and 12 months', () => {
  const rows = B.projectScenario(BASE, FULL, 'tendance');
  const ms = B.milestones(rows);
  assert.deepEqual(ms.map((r) => r.month), [0, 3, 6, 9, 12]);
  assert.deepEqual(B.milestones(rows, [3, 6, 12]).map((r) => r.month), [3, 6, 12]);
  assert.ok(Math.abs(B.milestones(rows, [3])[0].users - 1000 * 1.1 ** 3) < 1e-9);
});

test('empty assumptions give null, never 0', () => {
  const none = B.emptyAssumptions();
  const rows = B.projectScenario(BASE, none, 'tendance');
  assert.equal(rows[0].users, 1000, 'today is measured');
  for (let m = 1; m <= 12; m++) assert.equal(rows[m].users, null, 'no growth: no future');
  for (const r of rows) {
    assert.equal(r.actives, null);
    assert.equal(r.hours, null);
    assert.equal(r.peak, null);
    assert.equal(r.servers, null);
    assert.equal(r.bandwidthCost, null);
    assert.equal(r.revenue, null);
    assert.equal(r.costPerHour, null);
    assert.equal(r.costPerActive, null);
    assert.equal(r.costPartial, true);
  }
  // the server line is still measured: 1 machine
  assert.equal(rows[0].cost, 120);
  // each missing piece empties exactly what depends on it
  assert.equal(B.projectScenario(BASE, { ...FULL, share: null }, 'tendance')[3].hours, null);
  assert.equal(B.projectScenario(BASE, { ...FULL, hoursPerActive: null }, 'tendance')[3].hours, null);
  assert.equal(B.projectScenario(BASE, { ...FULL, growth: null }, 'tendance')[3].users, null);
  assert.equal(B.projectScenario(BASE, { ...FULL, tariff: null }, 'tendance')[12].revenue, null);
  assert.equal(B.projectScenario(BASE, { ...FULL, conversion: null }, 'tendance')[12].revenue, null);
  assert.equal(B.projectScenario(BASE, { ...FULL, streamLimit: null }, 'tendance')[12].servers, null);
  assert.equal(B.seriesOf(rows, 'users'), null);
  assert.equal(B.seriesOf(rows, 'hours'), null);
  assert.deepEqual(B.missingFor(none, B.USERS_NEEDS), ['Croissance mensuelle des inscrits']);
  assert.equal(B.missingFor(none, B.HOURS_NEEDS).length, 3);
});

test('an explicit 0 stays 0 (a value, not an empty field)', () => {
  const rows = B.projectScenario(BASE, { ...FULL, growth: 0 }, 'ambitieux');
  assert.equal(rows[12].users, 1000);
  const rev = B.projectScenario(BASE, { ...FULL, tariff: 0 }, 'tendance');
  assert.equal(rev[0].revenue, 0);
});

test('no division by zero anywhere: zero users, zero hours, zero peak, zero limit', () => {
  const cases = [
    [{ ...BASE, users: 0 }, FULL],
    [BASE, { ...FULL, share: 0 }],
    [BASE, { ...FULL, hoursPerActive: 0 }],
    [BASE, { ...FULL, streamLimit: 0 }],
    [{ ...BASE, peak: 0 }, FULL],
    [{ ...BASE, hoursPerMonth: 0 }, FULL],
    [{ ...BASE, serverMonthly: null, storageMonthly: null, bandwidthPerHour: null }, FULL],
  ];
  for (const [base, a] of cases) {
    for (const id of ['prudent', 'tendance', 'ambitieux']) {
      const rows = B.projectScenario(base, a, id);
      for (const r of rows) {
        for (const key of ['users', 'actives', 'hours', 'peak', 'servers', 'cost', 'costPerHour', 'costPerActive', 'revenue']) {
          assert.ok(finiteOrNull(r[key]), `${id} m${r.month} ${key} = ${r[key]}`);
        }
      }
      assert.ok(['unknown', 'already', 'at', 'none'].includes(B.saturation(rows, a.streamLimit).status));
    }
  }
  // zero hours at month 0: the peak cannot be scaled -> null, not NaN
  assert.equal(B.projectScenario(BASE, { ...FULL, share: 0 }, 'tendance')[5].peak, null);
  assert.equal(B.projectScenario(BASE, { ...FULL, share: 0 }, 'tendance')[5].costPerHour, null);
  // no cost line known at all: cost is null, not 0
  const nothing = B.projectScenario({ ...BASE, serverMonthly: null, storageMonthly: null, bandwidthPerHour: null }, FULL, 'tendance');
  assert.equal(nothing[0].cost, null);
});

test('saturation: unknown without a limit, already, at (interpolated), none', () => {
  const rows = B.projectScenario(BASE, FULL, 'tendance');
  assert.deepEqual(B.saturation(rows, null), { status: 'unknown', months: null });
  assert.equal(B.saturation(rows, 0).status, 'unknown');
  assert.equal(B.saturation(rows, 5).status, 'already');
  const at = B.saturation(rows, 20);
  assert.equal(at.status, 'at');
  // peak = 10 x 1.1^m reaches 20 at m = ln 2 / ln 1.1 = 7.27
  assert.ok(at.months > 6.5 && at.months < 7.5, String(at.months));
  assert.equal(B.saturation(rows, 100000).status, 'none');
  assert.equal(B.saturation(B.projectScenario(BASE, B.emptyAssumptions(), 'tendance'), 20).status, 'unknown');
  // a faster scenario saturates sooner
  const all = B.projectAll(BASE, FULL);
  assert.ok(B.saturation(all.ambitieux, 15).months < B.saturation(all.tendance, 15).months);
  assert.ok(B.saturation(all.tendance, 15).months < B.saturation(all.prudent, 15).months);
  assert.equal(B.saturation(all.prudent, 20).status, 'none', 'prudent: 5 % a month does not reach 20 in 12 months');
});

test('month labels and saturation texts are deterministic', () => {
  assert.equal(B.monthLabel('2026-10-05T11:16:17Z', 0), 'oct. 2026');
  assert.equal(B.monthLabel('2026-10-05T11:16:17Z', 3), 'janv. 2027');
  assert.equal(B.monthLabel('2026-10-05', 12, true), 'octobre 2027');
  assert.equal(B.monthLabel('nope', 2), 'M+2');
  assert.equal(B.monthLabels('2026-10-05').length, 13);
  assert.equal(B.saturationText({ status: 'unknown', months: null }, '2026-10-05').date, '[À MESURER]');
  assert.equal(B.saturationText({ status: 'none', months: null }, '2026-10-05').date, 'Après oct. 2027');
  assert.equal(nbsp(B.saturationText({ status: 'at', months: 7.27 }, '2026-10-05').detail), 'dans environ 7,3 mois');
  assert.equal(B.saturationText({ status: 'at', months: 7.27 }, '2026-10-05').date, 'mai 2027');
});

test('+ / - steps: empty starts at the default, nullable ones go back to empty, others stop at the floor', () => {
  const tariff = B.assumptionDef('tariff');
  assert.equal(B.stepAssumption(tariff, null, 1), 3);
  assert.equal(B.stepAssumption(tariff, null, -1), null);
  assert.equal(B.stepAssumption(tariff, 3, 1), 3.5);
  assert.equal(B.stepAssumption(tariff, 0.5, -1), null, 'below the minimum: empty again');
  const growth = B.assumptionDef('growth');
  assert.equal(B.stepAssumption(growth, 0.5, -1), 0);
  assert.equal(B.stepAssumption(growth, 0, -1), 0, 'a non-nullable one never becomes empty with "-"');
  assert.equal(B.stepAssumption(growth, 100, 1), 100);
  const cpg = B.assumptionDef('costPerGb');
  assert.equal(B.stepAssumption(cpg, 0.001, 1), 0.0015);
  assert.equal(B.stepAssumption(cpg, 0.0015, -1), 0.001, 'no floating point drift');
});

test('typed input: empty is null, comma decimals work, garbage is rejected', () => {
  assert.equal(B.parseAssumptionInput(''), null);
  assert.equal(B.parseAssumptionInput('  '), null);
  assert.equal(B.parseAssumptionInput('8,6'), 8.6);
  assert.equal(B.parseAssumptionInput('8.6'), 8.6);
  assert.equal(B.parseAssumptionInput('0'), 0);
  assert.equal(B.parseAssumptionInput('1 200'), 1200);
  assert.equal(B.parseAssumptionInput('8,'), 8, 'a number being typed');
  assert.equal(B.parseAssumptionInput('abc'), undefined);
  assert.equal(B.parseAssumptionInput('-3'), undefined);
  assert.equal(B.parseAssumptionInput('1e3'), undefined);
  assert.equal(B.parseAssumptionInput('1,2,3'), undefined);
  const g = B.assumptionDef('growth');
  assert.equal(B.clampAssumption(g, 500), 100);
  assert.equal(B.clampAssumption(g, 4), 4);
  assert.equal(B.assumptionEditText(g, 8.6), '8,6');
  assert.equal(B.assumptionEditText(g, null), '');
  assert.equal(B.assumptionText(B.assumptionDef('tariff'), null), '[TARIF]');
  assert.equal(B.assumptionText(B.assumptionDef('conversion'), null), '[À MESURER]');
  assert.equal(B.assumptionText(B.assumptionDef('costPerGb'), null), '[À RENSEIGNER]');
  assert.equal(nbsp(B.assumptionText(B.assumptionDef('hoursPerActive'), 6.4061)), '6,41');
});

test('cost breakdown from the real responses: monthly, partial, unmeasured lines stay null', () => {
  const costs = real('costs').data;
  const { baseline } = B.buildBaseline(costs, real('overview').data, 7);
  const bd = B.costBreakdown(costs, baseline);
  const byId = Object.fromEntries(bd.lines.map((l) => [l.id, l]));
  assert.ok(Math.abs(byId.server.value - 300) < 1e-9);
  assert.ok(Math.abs(byId.bandwidth.value - 2.5714285714) < 1e-6, '0.6 / 0.2333 per month');
  assert.equal(byId.storage.value, null);
  assert.equal(byId.storage.tag, '[À RENSEIGNER]');
  assert.match(byId.storage.detail, /GAZES_COST_STORAGE_PER_GB_MONTH/);
  assert.equal(byId.backup.value, null);
  assert.equal(bd.partial, true);
  assert.ok(Math.abs(bd.total - 302.5714285714) < 1e-6);
  assert.ok(byId.server.bar > 90);
  assert.equal(byId.storage.bar, 0);
  const none = B.costBreakdown(real('costs.empty').data, B.buildBaseline(real('costs.empty').data, real('overview.empty').data, 30).baseline);
  assert.equal(none.total === null || Number.isFinite(none.total), true);
});

test('findings: at most 3, urgent first, with no invented number', () => {
  const costs = real('costs').data;
  const { baseline } = B.buildBaseline(costs, real('overview').data, 7);
  const breakdown = B.costBreakdown(costs, baseline);
  const rows = B.projectScenario(BASE, FULL, 'tendance');
  const sat = B.saturation(rows, 20);
  const list = B.findings({ breakdown, assumptions: FULL, scenario: 'tendance', rows, saturation: sat, isoDate: '2026-10-05', peak: 10 });
  assert.ok(list.length <= 3 && list.length >= 1);
  assert.equal(list[0].id, 'saturation');
  assert.equal(list[0].level, 'moyenne', '7.3 months away');
  const none = B.emptyAssumptions();
  const unknown = B.findings({
    breakdown, assumptions: none, scenario: 'tendance', rows: B.projectScenario(BASE, none, 'tendance'),
    saturation: { status: 'unknown', months: null }, isoDate: '2026-10-05', peak: null,
  });
  assert.ok(unknown.some((f) => f.id === 'limite' && /pas mesurée/.test(f.title)));
  assert.ok(unknown.some((f) => f.id === 'revenu' && f.title.includes('[TARIF]') && f.title.includes('[À MESURER]')));
});

test('local persistence: round trip, tolerant reader', () => {
  const state = { scenario: 'prudent', edited: { growth: 4.5, tariff: null } };
  assert.deepEqual(B.parseStoredState(B.serializeState(state)), state);
  assert.equal(B.parseStoredState(null), null);
  assert.equal(B.parseStoredState(''), null);
  assert.equal(B.parseStoredState('{not json'), null);
  assert.equal(B.parseStoredState('42'), null);
  const dirty = B.parseStoredState(JSON.stringify({ scenario: 'zzz', edited: { growth: 'x', share: -1, tariff: 2, bogus: 1, conversion: null } }));
  assert.deepEqual(dirty, { scenario: 'tendance', edited: { tariff: 2, conversion: null } });
  const merged = B.mergeAssumptions({ ...B.emptyAssumptions(), growth: 8, share: 40 }, { growth: null, tariff: 3 });
  assert.equal(merged.growth, null, 'an emptied field stays emptied');
  assert.equal(merged.share, 40, 'untouched: measured default');
  assert.equal(merged.tariff, 3);
});

test('measured growth: proposed only with >= 50 users before and >= 14 days', () => {
  const costs = real('costs').data;
  const mk = (total, added) => {
    const o = structuredClone(real('overview').data);
    o.kpis.total_users.value = total;
    o.kpis.new_users.value = added;
    return o;
  };
  assert.equal(B.buildBaseline(costs, mk(52, 4), 30).measuredGrowth, null, '48 before');
  assert.equal(B.buildBaseline(costs, mk(60, 10), 13).measuredGrowth, null, '13 days');
  const ok = B.buildBaseline(costs, mk(60, 10), 30);
  assert.ok(Math.abs(ok.measuredGrowth - 20) < 1e-9, '10 / 50 over 30 days');
  assert.equal(ok.growthNote, null);
  const low = B.buildBaseline(costs, mk(52, 4), 30);
  assert.match(low.growthNote, /échantillon trop faible/i);
  // a large measured value is bounded
  assert.equal(B.buildBaseline(costs, mk(300, 200), 30).measuredGrowth, 100);
});

test('clampMonthlyGrowth bounds to [-50, +100] and the notice is short', () => {
  assert.equal(B.clampMonthlyGrowth(250), 100);
  assert.equal(B.clampMonthlyGrowth(-80), -50);
  assert.equal(B.clampMonthlyGrowth(12.5), 12.5);
  assert.equal(B.clampMonthlyGrowth(0), 0);
  assert.equal(B.clampMonthlyGrowth(null), null);
  assert.equal(B.clampMonthlyGrowth(Infinity), null);
  assert.equal(B.clampMonthlyGrowth(NaN), null);
  assert.equal(B.growthClampNotice(150), 'ramené à +100 %');
  assert.equal(B.growthClampNotice(50), null);
  assert.equal(B.clampAssumption(B.assumptionDef('growth'), 1e30), 100);
  assert.equal(B.mergeAssumptions(B.emptyAssumptions(), { growth: 900 }).growth, 100);
});

test('scenario multipliers apply after the bound and stay bounded', () => {
  assert.equal(B.scenarioGrowth(500, 'tendance'), 100);
  assert.equal(B.scenarioGrowth(500, 'ambitieux'), 100, '150 -> 100');
  assert.equal(B.scenarioGrowth(500, 'prudent'), 50, 'bound first (100), then x 0.5');
  assert.equal(B.scenarioGrowth(null, 'prudent'), null);
  for (const g of [-50, -20, 0, 5, 40, 100, 1000]) {
    const p = B.scenarioGrowth(g, 'prudent'), t = B.scenarioGrowth(g, 'tendance'), a = B.scenarioGrowth(g, 'ambitieux');
    assert.ok(p <= t && t <= a, `${g}: ${p} <= ${t} <= ${a}`);
    for (const v of [p, t, a]) assert.ok(v >= -50 && v <= 100);
  }
});

test('extreme inputs never give a non-finite or > 1e9 value; scenarios stay ordered', () => {
  const huge = { ...BASE, users: 5e8, peak: 1e6, hoursPerMonth: 1e9 };
  const extreme = [
    { ...FULL, growth: 1e300 },
    { ...FULL, growth: 100, share: 100, hoursPerActive: 200, tariff: 1000, conversion: 100, streamLimit: 1 },
    { ...FULL, growth: -1e9 },
    { ...FULL, growth: 100, gbPerHour: 20, costPerGb: 1 },
  ];
  for (const base of [BASE, huge]) {
    for (const a of extreme) {
      const all = B.projectAll(base, a);
      for (const id of ['prudent', 'tendance', 'ambitieux']) {
        for (const r of all[id]) {
          for (const key of ['users', 'actives', 'hours', 'peak', 'servers', 'serverCost', 'bandwidthCost', 'cost', 'costPerHour', 'costPerActive', 'revenue']) {
            const v = r[key];
            assert.ok(v === null || (Number.isFinite(v) && Math.abs(v) <= 1e9), `${id} m${r.month} ${key} = ${v}`);
          }
        }
      }
      for (const key of ['users', 'hours']) {
        const p = all.prudent[12][key], t = all.tendance[12][key], am = all.ambitieux[12][key];
        if (p !== null && t !== null && am !== null) assert.ok(p <= t && t <= am, `${key}: ${p} <= ${t} <= ${am}`);
      }
    }
  }
  assert.equal(B.intOr(Infinity), '[À MESURER]');
  assert.equal(B.decOr(NaN, 2), '[À MESURER]');
  // 5e8 users x 2^12 would overflow the cap: no value rendered
  assert.equal(B.projectScenario(huge, { ...FULL, growth: 100 }, 'tendance')[12].users, null);
});
