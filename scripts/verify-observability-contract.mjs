import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const schema = JSON.parse(await readFile('contracts/observability/v1/event.schema.json', 'utf8'))
const errors = JSON.parse(await readFile('contracts/observability/v1/error-codes.json', 'utf8'))
const fixture = JSON.parse(await readFile('contracts/observability/v1/example-stage-failed.json', 'utf8'))

assert.equal(schema.$schema, 'https://json-schema.org/draft/2020-12/schema')
for (const field of schema.required) assert.ok(Object.hasOwn(fixture, field), 'missing ' + field)
assert.match(fixture.eventId, /^evt_/)
assert.match(fixture.correlation.traceId, /^trc_/)
assert.ok(errors.some((item) => item.code === fixture.error.code))
assert.equal(new Set(errors.map((item) => item.code)).size, errors.length)
