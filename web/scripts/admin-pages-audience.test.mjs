// Pure logic of the Utilisateurs, Catalogue and Croissance pages, checked on the real recorded API responses.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { register } from 'node:module';
import { readFileSync } from 'node:fs';

register('data:text/javascript,' + encodeURIComponent(`
export async function resolve(specifier, context, next) {
  if (specifier.startsWith('.') && !/\\.[a-z]+$/.test(specifier)) {
    try { return await next(specifier + '.ts', context); } catch {}
  }
  return next(specifier, context);
}`));

const U = await import('../src/components/admin/pages/users.logic.ts');
const C = await import('../src/components/admin/pages/catalog.logic.ts');
const G = await import('../src/components/admin/pages/growth.logic.ts');
const L = await import('../src/components/admin/cards/cards.logic.ts');
const FN = await import('../src/components/admin/charts/funnel.logic.ts');
const Q = await import('../src/components/admin/charts/quadrant.logic.ts');

const real = (name) => JSON.parse(readFileSync(new URL(`../src/lib/admin/real/${name}.json`, import.meta.url), 'utf8'));
const nbsp = (s) => s.replace(/[  ]/g, ' ');

test('users: the directory state round-trips through the URL and invalid values fall back', () => {
  const d = U.parseUsersQuery({});
  assert.deepEqual(d, { q: '', sort: 'last_activity', dir: 'desc', page: 1, user: null });
  assert.equal(U.usersQueryString(d), '');
  const q = U.parseUsersQuery({ q: '  akira ', sort: 'sessions', dir: 'asc', page: '3', user: '12' });
  assert.deepEqual(q, { q: 'akira', sort: 'sessions', dir: 'asc', page: 3, user: 12 });
  assert.deepEqual(U.parseUsersQuery(Object.fromEntries(new URLSearchParams(U.usersQueryString(q)))), q);
  const bad = U.parseUsersQuery({ sort: 'drop table', dir: 'sideways', page: '-2', user: 'abc', q: 'x'.repeat(200) });
  assert.equal(bad.sort, 'last_activity');
  assert.equal(bad.dir, 'desc');
  assert.equal(bad.page, 1);
  assert.equal(bad.user, null);
  assert.equal(bad.q.length, 64);
  assert.equal(U.parseUsersQuery({ page: ['2', '5'] }).page, 2);
});

test('users: API parameters page by 25 and only send q when searching', () => {
  assert.deepEqual(U.usersApiParams(U.parseUsersQuery({ page: '3' })), { sort: 'last_activity', dir: 'desc', limit: 25, offset: 50 });
  assert.equal(U.usersApiParams(U.parseUsersQuery({ q: 'ak' })).q, 'ak');
  assert.equal(U.pageCount(0), 1);
  assert.equal(U.pageCount(25), 1);
  assert.equal(U.pageCount(26), 2);
});

test('users: status text, search echo and directory rows', () => {
  assert.equal(nbsp(U.directoryStatus({ total: 140, offset: 25, users: new Array(25).fill(0) }, false)), '26 à 50 sur 140 comptes');
  assert.equal(U.directoryStatus({ total: 0, offset: 0, users: [] }, true), 'Aucun compte ne correspond');
  assert.equal(L.sameQuery('akira ', 'akira'), true);
  assert.equal(L.sameQuery('', 'akira'), false);
  const list = real('users.session').data;
  const now = real('users.session').generated_at;
  const rows = list.users.map((u) => U.directoryRow(u, now));
  assert.equal(rows[0].pseudo, 'gina');
  assert.equal(rows[0].user_id, '#7');
  assert.equal(rows[0].last_activity, 'Jamais');
  assert.equal(rows[0].created_at, '29 mars 2026');
  const token = real('users.token').data.users[0];
  assert.equal(U.directoryRow(token, now).pseudo, '#7', 'without a pseudo the id identifies the account');
});

test('users: KPIs, insights and detail only use what the API sent', () => {
  const summary = real('users-summary').data;
  const kpis = U.usersKpis(summary);
  assert.equal(kpis.length, 5);
  assert.equal(kpis[0].value, 9);
  assert.equal(kpis[1].vs, 'vs 7 j préc.');
  assert.equal(nbsp(kpis[4].note), '33,3 % des inscrits');
  const insights = U.usersInsights(summary);
  assert.ok(insights.some((i) => i.level === 'haute' && /jamais lancé/.test(i.title)));
  assert.equal(U.usersInsights({ ...summary, kpis: { ...summary.kpis, total: { value: 0, previous: 0, delta_pct: null } } }).length, 0);

  const detail = real('user-detail.session').data;
  const now = real('user-detail.session').generated_at;
  const tiles = U.detailTiles(detail, undefined, now);
  assert.deepEqual(tiles.slice(0, 2).map((t) => t.value), ['#1', 'alice']);
  assert.ok(!tiles.some((t) => /mail/i.test(t.label)));
  const tokenDetail = real('user-detail.token').data;
  assert.ok(!U.detailTiles(tokenDetail, undefined, now).some((t) => t.label === 'Pseudo'));
  const hist = U.historyItems(detail, now);
  assert.equal(hist[0].status, 'Interrompu');
  assert.equal(hist[0].episode, 'Épisode 1');
  assert.equal(U.positionText(123.5), '2 min 04 s');
  assert.equal(U.progressItems(detail)[0].label, 'Alpha · Épisode 4');
  assert.equal(U.historyItems(real('user-detail.empty.session').data, now).length, 0);
});

test('catalog: tab parsing and the API format grouping', () => {
  assert.equal(C.parseCatalogFormat(undefined), 'all');
  assert.equal(C.parseCatalogFormat('movie'), 'movie');
  assert.equal(C.parseCatalogFormat('anime'), 'all');
  const all = real('catalog').data;
  assert.equal(C.totalSessions(all), 7);
  assert.equal(nbsp(String(C.formatShare(all, 'tv'))).slice(0, 5), '71.42');
  assert.equal(C.formatShare(all, 'all'), 100);
  assert.equal(C.formatShare({ ...all, formats: [] }, 'tv'), null);
});

test('catalog: KPIs keep unmeasured figures empty and flag leavers with a minimum volume', () => {
  const all = real('catalog').data;
  const kpis = C.catalogKpis(all);
  assert.equal(kpis.length, 6);
  assert.equal(kpis[0].value, 3);
  assert.equal(kpis[4].value, null);
  assert.equal(kpis[5].value, null);
  // Alpha (40 % complete, 5 sessions) is the only series with enough sessions: 60 % abandon.
  const lv = C.leavers(all);
  assert.deepEqual(lv.map((l) => l.title), ['Alpha']);
  assert.equal(lv[0].abandon, 60);
  const empty = real('catalog.empty').data;
  const emptyKpis = C.catalogKpis(empty);
  assert.equal(emptyKpis[1].value, null);
  assert.equal(emptyKpis[2].value, null);
  assert.equal(emptyKpis[3].value, null);
  assert.equal(C.catalogInsights(empty).length, 0);
});

test('catalog: the matrix thresholds give the same quadrants as the API (strictly above the median = high)', () => {
  const all = real('catalog').data;
  const m = C.matrix(all);
  const names = { topRight: 'safe', topLeft: 'push', bottomRight: 'watch', bottomLeft: 'retire' };
  const byTitle = Object.fromEntries(all.quadrant.points.map((p) => [p.title, p.quadrant_key]));
  for (const p of m.points) {
    assert.equal(names[Q.quadrantOf(p.x, p.y, m.xThreshold, m.yThreshold)], byTitle[p.label], p.label);
  }
  assert.ok(m.xMax >= Math.max(...m.points.map((p) => p.x)));
  assert.ok(m.yMin <= Math.min(...m.points.map((p) => p.y)));
  assert.ok(m.yMax <= 100);
});

test('catalog: tab table rows, summary and new series versus catalogue', () => {
  const all = real('catalog').data;
  const rows = C.topRows(all, C.totalSessions(all));
  assert.equal(rows[0].rank, '1');
  assert.equal(nbsp(String(rows[0].share)).slice(0, 5), '71.42');
  assert.equal(rows[1].completionTone, 'danger');
  assert.equal(rows[1].delta, null);
  assert.match(C.topSummary(real('catalog.movie').data, all), /Les films représentent/);
  const season = C.seasonCards(all);
  assert.equal(season.cards[0].series, '2');
  assert.equal(season.cards[1].perSeries.replace(/[  ]/g, ' '), '5,0');
  assert.match(season.note, /20 mars/);
  assert.equal(C.langName('jp'), 'Japonais');
  assert.equal(C.langName('xx'), 'XX');
  assert.equal(C.formatName('unknown'), 'Inconnu');
});

test('growth: KPIs use points for the stickiness and hide deltas without a previous value', () => {
  const g = real('growth').data;
  const kpis = G.growthKpis(g, 30);
  assert.equal(kpis.length, 4);
  assert.equal(kpis[0].delta, null, 'DAU previous is 0');
  assert.equal(kpis[2].delta, 400);
  assert.equal(kpis[3].deltaUnit, 'pt');
  assert.equal(kpis[3].delta, null, 'stickiness previous is 0');
  assert.equal(nbsp(kpis[1].note), '50,0 % des inscrits');
  assert.equal(G.totalUsers(g), 8);
  assert.equal(G.isNotMeasured(g, 'acquisition_sources'), true);
  assert.equal(G.isNotMeasured(g, 'dau'), false);
});

test('growth: too recent funnel steps are unmeasured and never the worst loss', () => {
  const g = real('growth').data;
  const steps = G.funnelSteps(g);
  assert.equal(steps.length, 5);
  assert.equal(steps[3].unmeasured, true);
  assert.match(steps[3].detail, /Trop récent/);
  assert.equal(steps[1].unmeasured, false);
  const f = FN.computeFunnel({ steps });
  assert.equal(f.rows[3].count, '—');
  assert.equal(f.rows[3].isWorst, false);
  assert.equal(f.rows[4].isWorst, false);
  assert.equal(f.rows[2].isWorst, true, 'three episodes lose 100 % of the 3 accounts with a session');
  assert.match(f.rows[2].convText, /de 3 comptes éligibles/);
  assert.match(f.ariaLabel, /Actif à J7 non mesurable/);
  // classic funnels keep their behaviour
  const plain = FN.computeFunnel({ steps: [{ label: 'A', value: 100 }, { label: 'B', value: 40 }, { label: 'C', value: 30 }] });
  assert.equal(plain.rows[1].isWorst, true);
  assert.match(plain.rows[2].convText, /de l’étape précédente/);
  // a measured step uses its eligible base, not the previous step
  const nested = FN.computeFunnel({ steps: [{ label: 'A', value: 10 }, { label: 'B', value: 8, eligible: 10 }, { label: 'C', value: 1, eligible: 2 }] });
  assert.equal(nested.rows[2].lossText.replace(/[  ]/g, ' '), '−1 (−50,0 %)');
  assert.equal(nested.worst, 2);
  const insights = G.growthInsights(g);
  assert.ok(insights.some((i) => /3 épisodes/.test(i.title)), JSON.stringify(insights));
  assert.ok(!insights.some((i) => /churn/i.test(i.title)), "no churn insight when nobody churned");
});

test('growth: cohorts keep null cells empty and weight the average by cohort size', () => {
  const g = real('growth').data;
  const c = G.cohortModel(g);
  assert.equal(c.rowLabels.length, g.cohorts.length + 1);
  assert.equal(c.rowLabels[0], '2 mars – 8 mars');
  assert.equal(c.values[0][0], null);
  assert.equal(c.texts[0][0], '');
  assert.equal(c.texts[2][1], '100 %');
  // J1 average: (0 * 1 + 33.33 * 3) / 4
  assert.equal(Math.round(c.values[g.cohorts.length][0]), 25);
  assert.equal(c.max, 100);
  assert.equal(c.hasData, true);
  assert.equal(G.cohortModel(real('growth.empty').data).hasData, false);
  assert.equal(G.addDays('2026-03-30', 6), '2026-04-05');
});

test('growth: delay, churn and cumulative blocks', () => {
  const g = real('growth').data;
  const d = G.delayModel(g);
  assert.equal(d.hasData, true);
  assert.equal(nbsp(d.tiles[1].value), '40,0 %');
  assert.equal(nbsp(d.tiles[2].value), '60,0 %');
  assert.equal(nbsp(d.tiles[0].value), '2,0 h', 'median signup → first session delay');
  assert.equal(G.delayModel(real('growth.empty').data).tiles[0].value, '', 'no account watched: [À MESURER]');
  assert.deepEqual([30, 600, 5400, 3 * 86400].map((s) => nbsp(G.delayText(s))), ['< 1 min', '10 min', '1,5 h', '3,0 j']);
  assert.equal(d.bars[4].tone, 'muted');
  const churn = G.churnModel(g);
  assert.equal(churn.measured, true);
  assert.equal(churn.value, 0);
  assert.equal(churn.tiles[3].value, '', 'no objective exists: no threshold');
  const none = G.churnModel({ ...g, churn: { ...g.churn, previous_window_active: 0, churned: 0, churn_pct: null } });
  assert.equal(none.measured, false);
  assert.equal(none.tiles[2].value, '');
  const cumul = G.cumulativeModel(g);
  assert.equal(cumul.values.length, 30);
  assert.equal(cumul.yMin, 0);
  assert.match(cumul.summary, /^\+5 sur la période$/);
  const weeks = G.weeksModel(g);
  assert.equal(weeks.best, 3);
  assert.equal(weeks.current, 1);
});
