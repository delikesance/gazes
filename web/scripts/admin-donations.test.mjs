// Pure logic of the Dons admin page.
import { test } from 'node:test';
import assert from 'node:assert/strict';

const D = await import('../src/components/admin/pages/donations.logic.ts');

const base = { id: 'a', provider: 'btcpay', provider_ref: 'inv', user_id: null, pseudo: null, donor_label: '', visibility: 'anonymous',
  display_name: '', amount_cents: 1250, currency: 'EUR', status: 'settled', message: '', created_at: 1_700_000_000, settled_at: 1_700_000_100 };

test('eur formats cents with a comma and no exotic spaces', () => {
  assert.equal(D.eur(1250), '12,50 €');
  assert.equal(D.eur(5), '0,05 €');
  assert.equal(D.eur(123456), '1 234,56 €');
  assert.equal(D.eur(100, 'USD'), '1,00 USD');
});

test('donorText shows the account and the label, admin side', () => {
  assert.equal(D.donorText(base), 'Donateur inconnu');
  assert.equal(D.donorText({ ...base, user_id: 7, pseudo: 'lea' }), 'lea (compte #7)');
  assert.equal(D.donorText({ ...base, user_id: 7, pseudo: 'lea', donor_label: 'Léa B.' }), 'lea (compte #7) · Léa B.');
  assert.equal(D.donorText({ ...base, donor_label: 'Bob' }), 'Bob');
});

test('publicText states what visitors see', () => {
  assert.equal(D.publicText(base), 'Anonyme');
  assert.equal(D.publicText({ ...base, visibility: 'named', display_name: 'Léa' }), 'Affiché : Léa');
  assert.equal(D.publicText({ ...base, status: 'pending', visibility: 'named', display_name: 'Léa' }), 'Pas encore public');
});

test('KPIs never invent an average', () => {
  const empty = D.donationKpis({ month_cents: 0, all_cents: 0, count: 0, donors: 0 });
  assert.equal(empty[3].value, null);
  const k = D.donationKpis({ month_cents: 500, all_cents: 1500, count: 3, donors: 2 });
  assert.equal(k[3].value, '5,00 €');
  assert.equal(k[2].note, '2 donateurs distincts');
});

test('row keeps amounts in euros and labels in French', () => {
  const r = D.donationRow({ ...base, provider: 'kofi' });
  assert.equal(r.amount, 12.5);
  assert.equal(r.provider, 'Ko-fi');
  assert.equal(r.status, 'Reçu');
  assert.equal(r.when, '2023-11-14 22:15');
});

test('euro and user id inputs', () => {
  assert.equal(D.parseEuroInput('12,5'), 1250);
  assert.equal(D.parseEuroInput('3'), 300);
  assert.equal(D.parseEuroInput('0'), null);
  assert.equal(D.parseEuroInput('1.234'), null);
  assert.equal(D.parseEuroInput('abc'), null);
  assert.equal(D.parseUserId(''), null);
  assert.equal(D.parseUserId('#12'), 12);
  assert.equal(D.parseUserId('x'), undefined);
  assert.equal(D.parseUserId('0'), undefined);
});
