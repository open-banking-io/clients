'use strict';

const { expect } = require('chai');
const schema = require('zapier-platform-schema');

const app = require('../index');

// Validates the full app definition against the real zapier-platform schema —
// the same check `zapier push` runs. Catches structural mistakes (e.g. arrays
// where keyed objects are required) that unit tests of individual operations
// cannot see.

function serialize(definition) {
  // Function properties (perform, test, …) are uploaded as FunctionSchema
  // objects ({ source }); a placeholder source is sufficient for validation.
  return JSON.parse(
    JSON.stringify(definition, (key, value) =>
      typeof value === 'function' ? { source: 'return [];' } : value,
    ),
  );
}

describe('app definition', () => {
  it('validates against zapier-platform-schema', () => {
    const results = schema.validateAppDefinition(serialize(app));
    const messages = results.errors.map((e) => `${e.property}: ${e.message}`);
    expect(messages).to.deep.equal([]);
  });

  it('keys every operation map by the operation key', () => {
    for (const [mapName, map] of Object.entries({
      triggers: app.triggers,
      searches: app.searches,
      creates: app.creates,
    })) {
      expect(map, mapName).to.be.an('object').and.not.an('array');
      for (const [key, op] of Object.entries(map)) {
        expect(op.key, `${mapName}.${key}`).to.equal(key);
      }
    }
  });

  it('points every dynamic dropdown at an existing trigger', () => {
    const triggerKeys = new Set(Object.keys(app.triggers));
    const ops = [...Object.values(app.searches), ...Object.values(app.creates), ...Object.values(app.triggers)];
    for (const op of ops) {
      for (const field of op.operation.inputFields ?? []) {
        if (field.dynamic) {
          const ref = field.dynamic.split('.')[0];
          expect(triggerKeys.has(ref), `dynamic ref "${field.dynamic}" on ${op.key}`).to.equal(true);
        }
      }
    }
  });
});
