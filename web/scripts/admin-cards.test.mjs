import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as L from '../src/components/admin/cards/cards.logic.ts';

const rows = [
  { id: 'a', name: 'Élodie', n: 10, nSort: 10, d: '2024-03-01', st: 'ok' },
  { id: 'b', name: 'zoé', n: 9, nSort: 2000, d: '2023-12-25', st: 'ko' },
  { id: 'c', name: 'Adam', n: null, d: '', st: 'ok' },
  { id: 'd', name: 'Bob', n: 100, nSort: undefined, d: '2024-01-10', st: 'ok' },
];
const ids = (r) => r.map((x) => x.id).join('');

test('format fr-FR and KPI value', () => {
  assert.equal(L.formatFr(1234.5, 1).replace(/\s/g, ' '), '1 234,5');
  assert.equal(L.formatFr(124300).replace(/\s/g, ' '), '124 300');
  assert.equal(L.formatFr(3.14159), '3,1');
  assert.deepEqual(L.formatKpiValue(null), { text: '[À MESURER]', missing: true });
  assert.deepEqual(L.formatKpiValue('12 h'), { text: '12 h', missing: false });
  assert.equal(L.formatKpiValue(8.25, 2).text, '8,25');
});

test('unit pluralisation', () => {
  assert.equal(L.pluralizeUnit('%', 5, 1), '%');
  assert.equal(L.pluralizeUnit('minute', 1, 0), 'minute');
  assert.equal(L.pluralizeUnit('minute', 2, 0), 'minutes');
  assert.equal(L.pluralizeUnit('heure', 3, 0, 'heures'), 'heures');
  assert.equal(L.pluralizeUnit('fois', 3, 0), 'fois');
  assert.equal(L.pluralizeUnit('j', 3, 0), 'j');
  assert.equal(L.pluralizeUnit('minute', 1.96, 1), 'minutes');
});

test('trend and clamp', () => {
  assert.deepEqual(L.formatTrend(12), { text: '+12 %', up: true });
  assert.deepEqual(L.formatTrend(-3.5), { text: '−3,5 %', up: false });
  assert.equal(L.clampPercent(140), 100);
  assert.equal(L.clampPercent(-4), 0);
});

test('sort: numeric, with separate sort value, empty last', () => {
  // n uses nSort when present: a=10, b=2000, d falls back to 100, c empty
  assert.equal(ids(L.sortRows(rows, 'n', 'asc')), 'adbc');
  assert.equal(ids(L.sortRows(rows, 'n', 'desc')), 'bdac');
});

test('sort: ISO dates and french text, empty last, stable', () => {
  assert.equal(ids(L.sortRows(rows, 'd', 'asc')), 'bdac');
  assert.equal(ids(L.sortRows(rows, 'd', 'desc')), 'adbc');
  assert.equal(ids(L.sortRows(rows, 'name', 'asc')), 'cdab');
  const eq = [{ id: '1', v: 1 }, { id: '2', v: 1 }, { id: '3', v: 1 }];
  assert.equal(ids(L.sortRows(eq, 'v', 'desc')), '123');
  assert.equal(ids(rows), 'abcd'); // input untouched
});

test('sort: natural numeric text order', () => {
  const r = [{ id: 'x', t: 'ép. 10' }, { id: 'y', t: 'ép. 2' }];
  assert.equal(ids(L.sortRows(r, 't', 'asc')), 'yx');
});

test('nextSort and aria-sort', () => {
  assert.deepEqual(L.nextSort({ key: 'n', dir: 'desc' }, { key: 'n' }), { key: 'n', dir: 'asc' });
  assert.deepEqual(L.nextSort({ key: 'x', dir: 'asc' }, { key: 'n', type: 'number' }), { key: 'n', dir: 'desc' });
  assert.deepEqual(L.nextSort({ key: 'x', dir: 'desc' }, { key: 'name', type: 'text' }), { key: 'name', dir: 'asc' });
  assert.equal(L.ariaSortOf(true, 'asc'), 'ascending');
  assert.equal(L.ariaSortOf(true, 'desc'), 'descending');
  assert.equal(L.ariaSortOf(false, 'asc'), 'none');
});

test('search is accent and case insensitive, handles object cells', () => {
  assert.equal(ids(L.filterBySearch(rows, 'elodie', ['name'])), 'a');
  assert.equal(ids(L.filterBySearch(rows, '  ZOE ', ['name'])), 'b');
  assert.equal(ids(L.filterBySearch(rows, '', ['name'])), 'abcd');
  assert.equal(ids(L.filterBySearch([{ id: 'q', c: { label: 'Résolu' } }], 'resolu', ['c'])), 'q');
  assert.equal(L.filterBySearch(rows, 'nope', ['name', 'st']).length, 0);
});

test('tabs filter and counters', () => {
  const tabs = [{ label: 'OK', value: 'ok' }, { label: 'KO', value: 'ko' }];
  assert.equal(ids(L.filterByTab(rows, 'st', 'ok')), 'acd');
  assert.equal(ids(L.filterByTab(rows, 'st', L.ALL_TAB)), 'abcd');
  assert.deepEqual(L.tabCounts(rows, 'st', tabs), { [L.ALL_TAB]: 4, ok: 3, ko: 1 });
});

test('count text', () => {
  assert.equal(L.countText(1, 1), '1 ligne');
  assert.equal(L.countText(5, 5), '5 lignes');
  assert.equal(L.countText(0, 0), '0 ligne');
  assert.equal(L.countText(2, 7), '2 sur 7 lignes');
});

test('cell formats', () => {
  assert.equal(L.formatNumberCell(1500, { unit: 'min' }).replace(/\s/g, ' '), '1 500 min');
  assert.equal(L.formatNumberCell(2.5, {}), '2,5');
  assert.deepEqual(L.formatDeltaCell(4.2, {}), { text: '+4,2 %', direction: 'up', tone: 'good' });
  assert.deepEqual(L.formatDeltaCell(-4.2, { goodWhen: 'down' }), { text: '−4,2 %', direction: 'down', tone: 'good' });
  assert.deepEqual(L.formatDeltaCell(0, { unit: '' }), { text: '0,0', direction: 'flat', tone: 'flat' });
  assert.equal(L.barMax(rows, { key: 'n' }), 100);
  assert.equal(L.barMax(rows, { key: 'n', max: 50 }), 50);
  assert.ok(L.barMax([], { key: 'n' }) > 0);
  assert.equal(L.barPercent(25, 100), 25);
  assert.equal(L.barPercent(250, 100), 100);
});

test('ids: row key fallback and unique search ids', () => {
  assert.equal(L.rowKeyOf({ id: 7 }, 3, 'id'), '7');
  assert.equal(L.rowKeyOf({}, 3, 'id'), '3');
  assert.equal(L.searchFieldId(':r1:'), 'dtr1-recherche');
  assert.equal(L.searchFieldId(':r1:', 'audit mcp'), 'dt-audit-mcp-recherche');
  assert.notEqual(L.searchFieldId(':r1:'), L.searchFieldId(':r2:'));
  assert.equal(L.columnAlign({ key: 'x', type: 'number' }), 'right');
  assert.equal(L.columnAlign({ key: 'x', type: 'chip' }), 'left');
});
