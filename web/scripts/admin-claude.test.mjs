// Pure logic of the "Claude et MCP" and "Paramètres" pages, checked on the real recorded API responses.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const L = await import('../src/components/admin/pages/claude.logic.ts');
const real = (name) => JSON.parse(readFileSync(new URL(`../src/lib/admin/real/${name}.json`, import.meta.url), 'utf8'));

test('tools are counted by autonomy level and every real tool has a known level', () => {
  const tools = real('mcp-tools').data.items;
  const counts = L.countByLevel(tools);
  assert.equal(Object.values(counts).reduce((a, b) => a + b, 0), tools.length);
  for (const t of tools) assert.ok(L.TOOL_LEVELS.includes(t.level), `unknown level ${t.level} for ${t.name}`);
  assert.ok(counts.read > 0);
});

test('labels fall back to the raw value instead of hiding it', () => {
  assert.equal(L.levelLabel('read'), 'Lecture');
  assert.equal(L.levelLabel('martian'), 'martian');
  assert.equal(L.outcomeLabel('budget_exceeded'), 'Budget dépassé');
  assert.equal(L.outcomeLabel('???'), '???');
  for (const o of ['error', 'denied', 'suspended', 'budget_exceeded']) assert.equal(L.OUTCOME_TONES[o], 'danger');
  assert.equal(L.tokenStatusLabel('revoked'), 'Révoqué');
});

test('pending approvals come first, oldest first; the others newest first', () => {
  const items = real('approvals').data.items;
  const sorted = L.sortApprovals(items);
  assert.equal(sorted.length, items.length);
  assert.equal(sorted[0].status, 'pending');
  const rest = sorted.slice(L.pendingCount(items));
  assert.deepEqual(rest.map((a) => a.id), [...rest.map((a) => a.id)].sort((a, b) => b - a));
  assert.equal(L.pendingCount(items), 1);
  assert.equal(L.approvalStatusLabel('pending'), 'En attente de décision');
});

test('argument previews are compact and bounded', () => {
  assert.equal(L.argsPreview({ rule: 'disk_pct', value: 90 }), '{"rule":"disk_pct","value":90}');
  assert.equal(L.argsPreview(null), '{}');
  const long = L.argsPreview({ note: 'x'.repeat(1000) }, 50);
  assert.equal(long.length, 50);
  assert.ok(long.endsWith('…'));
  const cyclic = {}; cyclic.self = cyclic;
  assert.equal(L.argsPreview(cyclic), '{…}');
});

test('the connection command never invents a host and rejects odd ones', () => {
  assert.match(L.connectionCommand(null), /\[HÔTE\]\/mcp/);
  assert.match(L.connectionCommand('https://gazes.example'), /gazes https:\/\/gazes\.example\/mcp /);
  assert.match(L.connectionCommand('https://gazes.example/'), /gazes https:\/\/gazes\.example\/mcp /);
  assert.match(L.connectionCommand('javascript:alert(1)'), /\[HÔTE\]/);
  assert.match(L.connectionCommand('https://a b'), /\[HÔTE\]/);
  assert.match(L.connectionCommand(null), /Bearer \[JETON\]/);
  assert.ok(L.TOKEN_CLI_COMMANDS.every((c) => c.startsWith('gazes-admin token ')));
});

test('audit rows keep the token as an id and never invent one', () => {
  const rows = real('mcp-audit').data.items.map(L.auditRowToTable);
  assert.ok(rows.length > 0);
  for (const r of rows) assert.ok(r.token === '—' || /^#\d+$/.test(r.token));
});

test('threshold input: comma accepted, bounds enforced, junk refused', () => {
  const t = { min: 50, max: 99 };
  assert.deepEqual(L.parseThresholdInput('90', t), { ok: true, value: 90 });
  assert.deepEqual(L.parseThresholdInput(' 87,5 ', t), { ok: true, value: 87.5 });
  for (const bad of ['', 'abc', '49.9', '99.1', '1e3', '--5', 'NaN', 'Infinity']) {
    assert.equal(L.parseThresholdInput(bad, t).ok, false, bad);
  }
  const real_ = real('settings').data.thresholds;
  assert.equal(real_.length, 4);
  for (const th of real_) {
    assert.equal(L.parseThresholdInput(String(th.default), th).ok, true, th.rule);
    assert.notEqual(L.ruleLabel(th.rule), th.rule);
    assert.ok(L.ruleUnit(th.rule) !== '');
  }
});

test('watch rules: labels, tones and values never turn "unknown" into zero', () => {
  const w = real('watch').data;
  assert.equal(w.rules.length, 5);
  assert.equal(L.formatWatchValue(null, '%'), '[À MESURER]');
  assert.equal(L.formatWatchValue(Number.NaN, '%'), '[À MESURER]');
  assert.equal(L.formatWatchValue(0, '%'), '0 %');
  assert.match(L.formatWatchValue(13.333, '%'), /^13,3\s%$/);
  assert.equal(L.ruleStateLabel('breached'), 'Dépassé');
  assert.equal(L.RULE_STATE_TONES.breached, 'danger');
  assert.equal(L.RULE_STATE_TONES.not_measured, 'muted');
  for (const r of w.rules) {
    assert.notEqual(L.ruleStateLabel(r.state), r.state, r.state);
    assert.notEqual(L.watchRuleLabel(r.rule), r.rule, r.rule);
    if (r.state === 'not_measured') assert.equal(r.value, null);
  }
  assert.equal(L.breachedRules(w.rules).length, 1);
});

test('effect verdicts all have a label and a tone', () => {
  for (const v of ['open', 'pending', 'improved', 'not_improved']) {
    assert.notEqual(L.verdictLabel(v), v);
    assert.ok(L.VERDICT_TONES[v]);
  }
  assert.equal(L.VERDICT_TONES.not_improved, 'danger');
  const empty = real('watch.empty').data;
  assert.equal(empty.effects.length, 0);
  assert.equal(L.breachedRules(empty.rules).length, 0);
});
