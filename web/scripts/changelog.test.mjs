import { test } from 'node:test';
import assert from 'node:assert/strict';
import { CHANGELOG, ANNOUNCE_DAYS, announcedEntry } from '../src/lib/changelog.ts';

const entry = (date) => ({ date, fr: { title: 'x', items: [] }, en: { title: 'x', items: [] } });
const at = (iso) => Date.parse(iso);

test('the latest entry is announced during its window', () => {
  const entries = [entry('2026-10-06'), entry('2026-10-01')];
  assert.equal(announcedEntry(entries, at('2026-10-06T08:00:00Z'), null)?.date, '2026-10-06');
  assert.equal(announcedEntry(entries, at('2026-10-19T23:00:00Z'), '2026-10-01')?.date, '2026-10-06');
});

test('no announcement once seen, expired, in the future or empty', () => {
  const entries = [entry('2026-10-06')];
  assert.equal(announcedEntry(entries, at('2026-10-07T00:00:00Z'), '2026-10-06'), null);
  assert.equal(announcedEntry(entries, at('2026-10-06T00:00:00Z') + (ANNOUNCE_DAYS * 86_400_000) + 1, null), null);
  assert.equal(announcedEntry(entries, at('2026-10-05T00:00:00Z'), null), null);
  assert.equal(announcedEntry([], Date.now(), null), null);
  assert.equal(announcedEntry([entry('nope')], Date.now(), null), null);
});

test('entries are newest first with unique valid dates', () => {
  const dates = CHANGELOG.map((e) => e.date);
  assert.ok(dates.length > 0);
  assert.equal(new Set(dates).size, dates.length);
  for (const d of dates) assert.match(d, /^\d{4}-\d{2}-\d{2}$/);
  assert.deepEqual([...dates].sort().reverse(), dates);
  for (const e of CHANGELOG) assert.equal(e.fr.items.length, e.en.items.length, e.date);
});
