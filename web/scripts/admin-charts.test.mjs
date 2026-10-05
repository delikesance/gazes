import { test } from 'node:test';
import assert from 'node:assert/strict';
import { pickStep, computeYAxis, buildLinePath, buildAreaPath, pickXTicks, computeLineChart } from '../src/components/admin/charts/line-chart.logic.ts';
import { computeBarChart } from '../src/components/admin/charts/bar-chart.logic.ts';
import { computeHBarList, formatDelta } from '../src/components/admin/charts/hbar-list.logic.ts';
import { cellOpacity, shownColumns, computeHeatmap } from '../src/components/admin/charts/heatmap.logic.ts';
import { computeLosses, computeFunnel } from '../src/components/admin/charts/funnel.logic.ts';
import { quadrantOf, computeQuadrant } from '../src/components/admin/charts/quadrant.logic.ts';
import { computeTimeline, formatDuration } from '../src/components/admin/charts/timeline.logic.ts';
import { computeSplitBarCard } from '../src/components/admin/charts/split-bar.logic.ts';

// toLocaleString('fr-FR') uses narrow no-break spaces: normalise before comparing.
const sp = (a) => a.map((s) => s.replace(/\s/g, ' '));

test('axis ticks: nice step, headroom and decimals', () => {
  assert.equal(pickStep(4137, 5), 1000);
  const a = computeYAxis({ data: [0, 3940], yTickCount: 4 });
  assert.deepEqual([a.yMin, a.top, a.step, a.intervals, a.decimals], [0, 5000, 1000, 5, 0]);
  assert.deepEqual(sp(a.ticks), ['5 000', '4 000', '3 000', '2 000', '1 000', '0']);
  const b = computeYAxis({ data: [0, 1] });
  assert.deepEqual([b.step, b.top, b.decimals], [0.25, 1.25, 2]);
  assert.deepEqual(b.ticks[0], '1,25');
  const c = computeYAxis({ data: [3, 40], yMax: 100, yTickCount: 4, unit: '%' });
  assert.deepEqual([c.yMin, c.top, c.step, c.intervals], [0, 100, 25, 4]);
  assert.deepEqual(sp(c.ticks), ['100 %', '75 %', '50 %', '25 %', '0 %']);
  assert.deepEqual(computeYAxis({ data: [] }).ticks.length > 1, true);
  assert.equal(computeYAxis({ data: [1, 2], decimals: 1, yMin: 0 }).ticks[0].includes(','), true);
});

test('line paths on 0, 1, 2 and 30 points, with gaps', () => {
  assert.equal(buildLinePath([], 0, 0, 10, 100), '');
  assert.equal(buildLinePath([5], 1, 0, 10, 100), 'M400.0 50.0');
  assert.equal(buildLinePath([0, 10], 2, 0, 10, 100), 'M0.0 100.0 L800.0 0.0');
  const vals = Array.from({ length: 30 }, (_, i) => i);
  const d = buildLinePath(vals, 30, 0, 30, 300);
  assert.equal((d.match(/M/g) ?? []).length, 1);
  assert.equal((d.match(/L/g) ?? []).length, 29);
  assert.ok(d.startsWith('M0.0 300.0 ') && d.endsWith('L800.0 10.0'));
  assert.equal(buildLinePath([1, null, 3], 3, 0, 4, 100), 'M0.0 75.0 M800.0 25.0');
  assert.equal(buildAreaPath([5], 1, 0, 10, 100), '');
  assert.equal(buildAreaPath([0, 10], 2, 0, 10, 100), 'M0.0 100.0 L800.0 0.0 L800.0 100 L0.0 100 Z');
  assert.deepEqual(pickXTicks(['a', 'b', 'c', 'd', 'e', 'f', 'g'], 3), ['a', 'd', 'g']);
  assert.deepEqual(pickXTicks(['a', 'b'], 5), ['a', 'b']);
});

test('line chart model: range marks, legend and aria label', () => {
  const m = computeLineChart({
    title: 'Vues',
    series: [{ label: 'A', values: Array.from({ length: 10 }, (_, i) => i * 10) }, { label: 'B', values: [1, 2], style: 'dashed' }],
    xLabels: ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10'],
    marks: [{ index: 2, to: 4, label: 'Panne', tone: 'danger', cause: 'DB' }, { index: 99, label: 'x' }],
    area: true,
  });
  assert.equal(m.legend, true);
  assert.equal(m.bands.length, 1);
  assert.equal(m.bands[0].danger, true);
  assert.equal(m.bands[0].leftPct.toFixed(2), '22.22');
  assert.equal(m.marks[0].range, 'Du 3 au 5');
  assert.equal(m.marks[0].cause, 'Cause : DB');
  assert.equal(m.marks[1].leftPct, 100);
  assert.equal(m.series[1].dash, '6 5');
  assert.ok(m.areaPath.endsWith('Z') && m.areaFill === '#9b8afb');
  assert.match(m.ariaLabel, /^Vues : A de 0 à 90 \(min 0, max 90\) ; B de 1 à 2/);
  assert.equal(computeLineChart({ series: [] }).legend, false);
});

test('bar chart: null values become 2px stubs, max highlighted', () => {
  const m = computeBarChart({ values: [null, 5, 10], height: 200, labels: ['a', 'b', 'c'] });
  assert.deepEqual(m.bars.map((b) => b.height), [2, 100, 200]);
  assert.equal(m.each, true);
  assert.equal(m.legend, false);
  const h = computeBarChart({ values: [1, 9, 3], highlight: 'max', height: 100 });
  assert.deepEqual(h.bars.map((b) => [b.highlighted, b.dimmed]), [[false, true], [true, false], [false, true]]);
  assert.equal(h.bars[1].val, '9');
  assert.equal(h.bars[1].height, 80);
  assert.equal(h.legend, true);
  const e = computeBarChart({ values: [], });
  assert.deepEqual([e.bars.length, e.ends], [0, false]);
  const many = computeBarChart({ values: Array.from({ length: 30 }, () => 1), labels: Array.from({ length: 30 }, (_, i) => 'd' + i), labelMode: 'ends', labelEvery: 10 });
  assert.deepEqual(many.bars.map((b) => b.every).filter(Boolean), ['d0', 'd10', 'd20']);
  assert.equal(many.gap, 2);
  assert.equal(many.ends, false);
});

test('horizontal bar list: sort, limit, deltas', () => {
  const m = computeHBarList({
    items: [{ label: 'a', value: 5, delta: 2 }, { label: 'b', value: 10, delta: -3.456 }, { label: 'c', value: null, delta: '[À MESURER]' }],
    sort: 'desc', limit: 2,
  });
  assert.deepEqual(m.rows.map((r) => r.label), ['b', 'a']);
  assert.deepEqual(m.rows.map((r) => r.widthPct), [100, 50]);
  assert.equal(m.rows[0].deltaDir, 'down');
  assert.equal(m.rows[0].delta.replace(/\s/g, ' '), '−3,5 %');
  const t = computeHBarList({ items: [{ label: 'c', value: null, delta: '[À MESURER]' }] });
  assert.deepEqual([t.rows[0].delta, t.rows[0].deltaDir, t.rows[0].widthPct], ['[À MESURER]', null, 0]);
  assert.equal(formatDelta(3, 'point', undefined), '+3 points');
  assert.equal(formatDelta(1, 'point', undefined), '+1 point');
  assert.equal(computeHBarList({ items: [] }).rows.length, 0);
});

test('heatmap: opacity steps, column labels, missing cells', () => {
  assert.equal(cellOpacity(0), 0.1);
  assert.equal(cellOpacity(1), 1);
  assert.equal(cellOpacity(5), 1);
  assert.deepEqual([...shownColumns(24, 6, 0)], [0, 5, 9, 14, 18, 23]);
  assert.deepEqual([...shownColumns(10, 6, 3)], [0, 3, 6, 9]);
  const m = computeHeatmap({ rowLabels: ['Lun', 'Mar'], colLabels: ['0h', '1h'], values: [[0.5, null], [1, 0]], showValues: true });
  assert.equal(m.rows[0].cells[1].missing, true);
  assert.equal(m.rows[0].cells[1].text, '—');
  assert.equal(m.rows[1].cells[0].text, '100 %');
  assert.equal(m.rows[1].cells[0].darkText, true);
  assert.equal(m.missingNote, '« — » : valeur non mesurable.');
  assert.match(m.ariaLabel, /valeur la plus élevée : Mar à 0h \(100 %\)/);
});

test('funnel: largest relative loss is flagged', () => {
  const l = computeLosses([1000, 600, 540, 100]);
  assert.equal(l.worst, 3);
  assert.ok(Math.abs(l.worstLoss - 440 / 540) < 1e-12);
  assert.equal(computeLosses([10, 10, 10]).worst, -1);
  assert.equal(computeLosses([100, 50, 100, 50]).worst, 1, 'ties keep the first step');
  const f = computeFunnel({ steps: [{ label: 'A', value: 1000 }, { label: 'B', value: 600 }, { label: 'C', value: 540 }, { label: 'D', value: 100 }] });
  assert.deepEqual(f.rows.map((r) => r.isWorst), [false, false, false, true]);
  assert.equal(f.rows[0].lossText, null);
  assert.equal(f.rows[3].lossText.replace(/\s/g, ' '), '−440 (−81,5 %)');
  assert.equal(f.rows[3].widthPct, 10);
  assert.match(f.ariaLabel, /Plus forte perte : de C à D/);
  assert.equal(computeFunnel({ steps: [{ label: 'A', value: 5 }, { label: 'B', value: 1 }], highlightLoss: false }).rows[1].isWorst, false);
  assert.equal(computeFunnel({ steps: [] }).rows.length, 0);
});

test('quadrants: threshold on the mean, boundary goes up/right', () => {
  assert.equal(quadrantOf(50, 50, 50, 50), 'topRight');
  assert.equal(quadrantOf(49, 50, 50, 50), 'topLeft');
  assert.equal(quadrantOf(50, 49, 50, 50), 'bottomRight');
  assert.equal(quadrantOf(0, 0, 50, 50), 'bottomLeft');
  const m = computeQuadrant({
    points: [{ label: 'A', x: 10, y: 10 }, { label: 'B', x: 90, y: 90, size: 4 }, { label: 'C', x: 10, y: 90 }, { label: 'D', x: 90, y: 10, size: 2 }],
    xMin: 0, xMax: 100, yMin: 0, yMax: 100,
  });
  assert.deepEqual([m.thrX, m.thrY], [50, 50]);
  assert.deepEqual(m.quadrants.map((q) => [q.key, q.titles]), [['topLeft', 'C'], ['topRight', 'B'], ['bottomLeft', 'A'], ['bottomRight', 'D']]);
  assert.deepEqual(m.points.map((p) => p.diameter), [12, 20, 12, 14]);
  assert.deepEqual(m.points.map((p) => p.flip), [false, true, false, true]);
  assert.deepEqual(m.points[1], { ...m.points[1], leftPct: 90, bottomPct: 90, quadrant: 'topRight' });
  assert.deepEqual(m.xTicks.length, 5);
  assert.equal(m.quadrants[0].n, '1 élément');
  const e = computeQuadrant({ points: [] });
  assert.deepEqual(e.quadrants.map((q) => q.titles), ['Aucun', 'Aucun', 'Aucun', 'Aucun']);
  assert.equal(computeQuadrant({ points: [{ label: 'A', x: 1, y: 1 }], quadrantNames: { topRight: 'Top' } }).quadrants[1].name, 'Top');
});

test('timeline: strip, window and ordering', () => {
  const m = computeTimeline({
    endDate: '2026-10-05',
    incidents: [
      { date: '2026-10-04', title: 'Basse', severity: 'basse' },
      { date: '2026-10-04', title: 'Haute', severity: 'haute', duration: 125, time: '10:00' },
      { date: '2026-10-01', title: 'Moy', severity: 'moyenne' },
      { date: '2026-08-01', title: 'Hors fenêtre', severity: 'haute' },
    ],
  });
  assert.equal(m.strip.length, 30);
  assert.equal(m.firstDay, '6 sept.');
  assert.equal(m.lastDay, '5 oct.');
  assert.deepEqual([m.strip[28].height, m.strip[28].color], [44, '#f87171']);
  assert.equal(m.strip[29].height, 6);
  assert.deepEqual(m.incidents.map((i) => i.title), ['Basse', 'Haute', 'Moy']);
  assert.equal(m.incidents[1].meta, '10:00 · 2 h 05');
  assert.deepEqual(m.legend.map((g) => g.label), ['Haute : 1', 'Moyenne : 1', 'Basse : 1']);
  assert.equal(m.gap, 3);
  assert.equal(computeTimeline({ incidents: [], days: 3 }).strip.length, 7);
  assert.equal(computeTimeline({ incidents: [], days: 90, endDate: '2026-10-05' }).gap, 1);
  assert.equal(formatDuration(45), '45 min');
  assert.equal(formatDuration('2 h'), '2 h');
  const empty = computeTimeline({ incidents: [], today: new Date(Date.UTC(2026, 0, 31)) });
  assert.deepEqual([empty.incidents.length, empty.lastDay], [0, '31 janv.']);
});

test('split bar card: shares, tone cycle and groups', () => {
  const m = computeSplitBarCard({ segments: [{ label: 'FR', value: 3 }, { label: 'EN', value: 1 }, { label: 'xx', value: null }] });
  assert.deepEqual(m.groups[0].segs.map((s) => s.widthPct), [75, 25, 0]);
  assert.deepEqual(m.groups[0].segs.map((s) => s.hasValue), [true, true, false]);
  assert.deepEqual(m.groups[0].segs.map((s) => s.color), ['#9b8afb', '#fafafa', '#52525b']);
  assert.equal(m.groups[0].segs[0].text.replace(/\s/g, ' '), 'FR 75 %');
  const g = computeSplitBarCard({ groups: [{ label: 'Mobile', segments: [{ label: 'a', value: 2 }] }], showValue: 'raw', unit: 'h' });
  assert.deepEqual([g.groups[0].label, g.groups[0].total, g.groups[0].segs[0].text], ['Mobile', '2 h', 'a 2 h']);
  assert.equal(computeSplitBarCard({ segments: [{ label: 'z', value: 0 }] }).groups[0].segs[0].widthPct, 0);
});
