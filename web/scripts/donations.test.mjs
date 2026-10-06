// Pure helpers of the public donation page.
import { test } from 'node:test';
import assert from 'node:assert/strict';

const D = await import('../src/lib/donations.ts');

test('eur drops the cents when round', () => {
  assert.equal(D.eur(500), '5 €');
  assert.equal(D.eur(1250), '12,50 €');
  assert.equal(D.eur(123400), '1 234 €');
});

test('parseAmount accepts only positive amounts with at most two decimals', () => {
  assert.equal(D.parseAmount('5'), 500);
  assert.equal(D.parseAmount('2,5'), 250);
  assert.equal(D.parseAmount(' 7.05 '), 705);
  for (const bad of ['', '0', '-1', '1.234', 'abc', '1e3', '1 000']) assert.equal(D.parseAmount(bad), null, bad);
});

test('validName mirrors the server rule', () => {
  for (const ok of ['Léa', 'Jean  Pierre', "o'neil", '夜月']) assert.ok(D.validName(ok), ok);
  for (const bad of ['', 'a', 'x'.repeat(25), '<b>', 'http://x', 'www.x', '@bob', 'a.b']) assert.ok(!D.validName(bad), bad);
});

test('goalPercent is null without a goal and capped at 100', () => {
  assert.equal(D.goalPercent({}), null);
  assert.equal(D.goalPercent({ goal_cents: 5000 }), null);
  assert.equal(D.goalPercent({ goal_cents: 5000, month_cents: 1250 }), 25);
  assert.equal(D.goalPercent({ goal_cents: 5000, month_cents: 99999 }), 100);
});
