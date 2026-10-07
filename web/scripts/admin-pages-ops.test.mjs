// Pure logic of the Vue d'ensemble, Visionnages and Lecteur et flux pages, checked on the real recorded API responses.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { register } from 'node:module';
import { readFileSync } from 'node:fs';

// The logic files import each other without extension (bundler resolution): teach node to find the .ts.
register('data:text/javascript,' + encodeURIComponent(`
export async function resolve(specifier, context, next) {
  if (specifier.startsWith('.') && !/\\.[a-z]+$/.test(specifier)) {
    try { return await next(specifier + '.ts', context); } catch {}
  }
  return next(specifier, context);
}`));

const F = await import('../src/components/admin/pages/fr-date.ts');
const O = await import('../src/components/admin/pages/overview.logic.ts');
const V = await import('../src/components/admin/pages/views.logic.ts');
const P = await import('../src/components/admin/pages/playback.logic.ts');

const real = (name) => JSON.parse(readFileSync(new URL(`../src/lib/admin/real/${name}.json`, import.meta.url), 'utf8'));
const nbsp = (s) => s.replace(/[  ]/g, ' ');

test('fr-date formatting is deterministic', () => {
  assert.equal(F.dayLabel('2026-03-05'), '5 mars');
  assert.equal(F.rangeLabel('2026-03-02', '2026-03-31'), 'Du 2 mars au 31 mars 2026');
  assert.equal(F.relativeFr('2026-10-05T11:10:00Z', '2026-10-05T11:16:17Z'), 'il y a 6 min');
  assert.equal(F.relativeFr('2026-10-05T09:00:00Z', '2026-10-05T11:16:17Z'), 'il y a 2 h');
  assert.equal(F.dateTimeFr('2026-03-31T08:00:00Z'), '31 mars 2026, 08:00 UTC');
  assert.equal(F.tzLabel(60), 'UTC+1');
  assert.equal(F.tzLabel(-240), 'UTC−4');
  assert.equal(F.tzLabel(330), 'UTC+5:30');
  assert.equal(F.tzLabel(0), 'UTC');
  assert.equal(nbsp(F.fmtDec(1234.56, 1)), '1 234,6');
  assert.equal(F.pointsDelta(30, 0), null);
  assert.equal(F.pointsDelta(30, 25), 5);
});

test('overview: KPIs keep unmeasured deltas hidden and use points for the completion rate', () => {
  const ov = real('overview').data;
  const kpis = O.overviewKpis(ov);
  assert.equal(kpis.length, 6);
  assert.equal(kpis.find((k) => k.label === 'Épisodes terminés').delta, null); // previous was 0
  assert.equal(kpis.find((k) => k.label === 'Visionnages').series.length, ov.series.length);
  const empty = O.overviewKpis(real('overview.empty').data);
  assert.ok(empty.every((k) => k.delta === null));
});

test('overview: top anime, signups, insights, playback tiles', () => {
  const ov = real('overview').data;
  const top = O.topAnimeRows(ov);
  assert.equal(top[0].rank, 1);
  assert.equal(top[0].bar, 100);
  assert.ok(top.every((t) => t.bar <= 100));
  const s = O.signupsBlock(ov);
  assert.equal(s.total, ov.series.reduce((a, d) => a + d.new_users, 0));
  const health = real('playback-health').data;
  const insights = O.overviewInsights(ov, health);
  assert.ok(insights.length > 0 && insights.length <= 3);
  assert.equal(insights[0].level, 'haute'); // 41,7 % error rate in the fixture
  assert.deepEqual(O.overviewInsights(real('overview.empty').data, null), []);
  const tiles = O.playbackTiles(real('playback-health.empty').data);
  assert.deepEqual(tiles, { activeSessions: '', startup: '', errorRate: '' }); // empty string = [À MESURER]
  assert.equal(O.playbackTiles(health).activeSessions, '7');
  const errs = O.latestErrors(real('playback-errors').data, '2026-10-05T11:16:17Z');
  assert.equal(errs.length, 3);
  assert.match(errs[0].text, /Anime n° 100 · ép\. 1 · nyaa\.si/);
  assert.deepEqual(O.latestErrors(null, ''), []);
});

test('views: median and split of distinct episodes per active user', () => {
  const e = V.episodesDistribution(real('views').data);
  assert.equal(e.hasData, true);
  assert.equal(nbsp(e.median), '1,5 épisodes');
  assert.deepEqual(e.bars.map((b) => [b.key, b.value]), [['1', 50], ['2_5', 50], ['6_12', 0], ['13_plus', 0]]);
  assert.equal(nbsp(e.bars[0].text), '2 · 50,0 %');
  const empty = V.episodesDistribution(real('views.empty').data);
  assert.deepEqual([empty.hasData, empty.median], [false, ''], 'no active user: [À MESURER]');
});

test('views: durations, retention, hours, timezones, leavers', () => {
  const v = real('views').data;
  const d = V.durationBars(v);
  assert.equal(d.labels[0], '< 5 min');
  const r = V.retention(v);
  assert.equal(r.values.length, 10);
  assert.ok(r.marks.every((m) => m.to === m.index + 1));
  const h = V.hourBars(v);
  assert.equal(h.values.length, 24);
  assert.ok(Math.abs(h.values.reduce((a, b) => a + b, 0) - 100) < 1e-6);
  assert.equal(h.peak.length, 4);
  assert.equal(V.timezoneItems(v)[0].label, 'UTC+1');
  assert.equal(V.leaverRows(v)[0].ep, 'Épisode 1');
  const e = real('views.empty').data;
  assert.equal(V.hourBars(e).peak.length, 0);
  assert.equal(V.retention(e).hasData, false);
  assert.equal(V.resumeBlock(e).hasData, false);
  assert.deepEqual(V.viewsInsights(e), []);
  const kpis = V.viewsKpis(e);
  assert.equal(kpis[1].value, null); // average duration with no session: [À MESURER], not 0
});

test('playback: KPI, findings, journal and copy context', () => {
  const health = real('playback-health').data;
  const summary = real('playback-errors-summary').data;
  const sources = real('playback-sources').data;
  const errors = real('playback-errors').data;
  const kpis = P.playbackKpis(health, summary, real('costs.partial').data);
  assert.equal(kpis.find((k) => k.label === 'Démarrage médian').value, null);
  assert.equal(kpis.find((k) => k.label === 'Cache Redis').value, 'Désactivé');
  assert.equal(kpis.find((k) => k.label === 'Erreurs de lecture').goodDown, true);

  const codes = P.errorsByCode(summary);
  assert.equal(codes[0].label, 'STREAM_TIMEOUT');
  assert.ok(codes.every((c) => c.value > 0));

  const rows = P.journalRows(errors.items, null);
  assert.equal(rows.length, errors.items.length);
  assert.equal(rows[2].show, '—');
  assert.equal(rows[2].src, '—');
  assert.equal(rows[0].ctx.label, 'Copier le contexte');
  const copied = P.journalRows(errors.items, { id: rows[1].id, status: 'copied' });
  assert.equal(copied[1].ctx.label, 'Copié');
  assert.equal(copied[0].ctx.label, 'Copier le contexte');
  const failed = P.journalRows(errors.items, { id: rows[1].id, status: 'failed' });
  assert.equal(failed[1].ctxTone, 'danger');

  const tabs = P.journalTabs(errors.items, '__all__');
  assert.equal(tabs[0].label, 'Tous (5)');
  assert.equal(tabs[1].value, 'STREAM_TIMEOUT');

  const text = P.copyText(errors.items[1], summary);
  assert.match(text, /Code : STREAM_TIMEOUT/);
  assert.match(text, /Anime : n° 100/);
  assert.match(text, /Cause probable : /);
  assert.match(P.copyText(errors.items[2], summary), /Anime : inconnu/);

  const findings = P.allFindings(health, summary, sources, real('issues').data, 30);
  assert.ok(findings.length >= 3 && findings.length <= 6);
  assert.equal(findings[0].level, 'haute');
  assert.ok(findings.some((f) => f.title === 'Sources lentes'));

  const gaps = P.notMeasured(health, sources, real('costs.partial').data).map((g) => g.label);
  assert.ok(gaps.includes('Torrents actifs') && gaps.includes('Chronologie d\'incidents') && gaps.includes('Stockage disque'));
  assert.equal(P.cacheBlock(health).tiles.find((t) => t.label === 'Évictions').value, '');
  assert.equal(P.cacheBlock(real('playback-health.empty').data).subtitle, '[À MESURER]');
});
