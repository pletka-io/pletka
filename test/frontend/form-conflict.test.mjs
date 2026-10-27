import { test } from 'node:test';
import assert from 'node:assert/strict';
import { planFormConflict, resolveFormConflict } from '../../frontend/src/lib/components/form/form-conflict.ts';

function schema(values, identity = 'draft-and-sources') {
  return { entity_type: 'arbitrary', mode: 'edit', endpoint: { method: 'PUT', url: '/provided/by/server' }, conflict: { revision_field: 'revision', identity, message: { en: 'Changed' } }, sections: [{ id: 'data', label: {}, fields: Object.entries(values).map(([name, value]) => ({ name, value, widget: name === 'revision' ? 'hidden' : 'text', label: { en: name } })) }], ui: { languages: [], primary_language: 'en' } };
}
test('independent changes merge without mutating unsaved values or advancing the revision early', () => {
  const base = { revision: 1, name: 'Original', source: 'A' };
  const mine = { ...base, source: 'B' };
  const latest = { ...base, revision: 2, name: 'Remote' };
  const plan = planFormConflict(schema(base), base, mine, schema(latest));
  assert.deepEqual(plan.conflicts, []);
  assert.deepEqual(resolveFormConflict(plan, {}), { revision: 2, name: 'Remote', source: 'B' });
  assert.deepEqual(mine, { revision: 1, name: 'Original', source: 'B' });
  assert.equal(base.revision, 1);
});
test('overlapping edits require explicit choices and retain both complete sources', () => {
  const base = { revision: 1, source: '<original/>', description: { en: 'Name', nl: 'Naam' } };
  const mine = { ...base, source: '<my edit/>' };
  const latest = { ...base, revision: 2, source: '<their edit/>', description: { nl: 'Naam', en: 'Name' } };
  const plan = planFormConflict(schema(base), base, mine, schema(latest));
  assert.equal(plan.conflicts.length, 1);
  assert.throws(() => resolveFormConflict(plan, {}), /Choose a value/);
  assert.equal(resolveFormConflict(plan, { source: 'mine' }).source, '<my edit/>');
  assert.equal(resolveFormConflict(plan, { source: 'latest' }).source, '<their edit/>');
  assert.equal(mine.source, '<my edit/>');
});
test('equal overlapping changes are safe; identical revisions do not invent a newer revision', () => {
  const base = { revision: 1, name: 'Old' }, mine = { ...base, name: 'Same' };
  const plan = planFormConflict(schema(base), base, mine, schema({ revision: 2, name: 'Same' }));
  assert.equal(plan.conflicts.length, 0);
  assert.equal(resolveFormConflict(plan, {}).name, 'Same');
});
test('changed source ordering, endpoint, fields, permissions or invalid revisions refuse rebase', () => {
  const base = { revision: 2, name: 'Original' }, mine = { ...base, name: 'Unsaved' };
  const variants = [schema({ ...base, revision: 3 }, 'different-sources'), schema({ revision: 3, other: '' }), schema({ ...base, revision: 1 }), schema({ ...base, revision: '3' })];
  const retarget = schema({ ...base, revision: 3 }); retarget.endpoint.url = '/other-owner'; variants.push(retarget);
  const readonly = schema({ ...base, revision: 3 }); delete readonly.endpoint; variants.push(readonly);
  for (const latest of variants) assert.throws(() => planFormConflict(schema(base), base, mine, latest));
  assert.deepEqual(mine, { revision: 2, name: 'Unsaved' });
});

test('empty widget values use the same defaults as initial form loading', () => {
  const base = { revision: 1, description: {}, name: 'Old' };
  const mine = { ...base, description: { en: 'My description' } };
  const latest = { revision: 2, description: null, name: 'Remote' };
  const plan = planFormConflict(schema(base), base, mine, schema(latest), () => ({}));
  assert.equal(plan.conflicts.length, 0);
  assert.deepEqual(resolveFormConflict(plan, {}).description, { en: 'My description' });
});
