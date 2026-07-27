import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const schema = JSON.parse(await readFile('contracts/observability/v1/event.schema.json', 'utf8'))
const errors = JSON.parse(await readFile('contracts/observability/v1/error-codes.json', 'utf8'))
const fixture = JSON.parse(await readFile('contracts/observability/v1/example-stage-failed.json', 'utf8'))

const approvedCodes = [
  'AUTH.SESSION.EXPIRED',
  'WORKFLOW.STAGE.TIMEOUT',
  'AGENT.PLAN.VALIDATION_FAILED',
  'LLM.PROVIDER.RATE_LIMITED',
  'LLM.RESPONSE.SCHEMA_INVALID',
  'MCP.CONNECTION.UNAVAILABLE',
  'MCP.TOOL.NOT_FOUND',
  'TOOL.ARGUMENT.SCHEMA_INVALID',
  'ARTIFACT.FILE.MISSING',
  'ARTIFACT.HASH.MISMATCH',
  'RENDER.BLENDER.PROCESS_FAILED',
  'RENDER.FFMPEG.CODEC_UNSUPPORTED',
  'MEDIA.AUDIO.DURATION_MISMATCH'
]

const typeMatches = (value, type) => ({
  array: Array.isArray(value),
  boolean: typeof value === 'boolean',
  integer: Number.isInteger(value),
  null: value === null,
  number: typeof value === 'number' && Number.isFinite(value),
  object: value !== null && typeof value === 'object' && !Array.isArray(value),
  string: typeof value === 'string'
}[type])

const resolve = (definition) => {
  if (!definition.$ref) return definition
  const path = definition.$ref.split('/').slice(1)
  return path.reduce((value, key) => value[key], schema)
}

function validate(value, definition, path = '$') {
  const rules = resolve(definition)
  const issues = []
  if (rules.oneOf) {
    const matches = rules.oneOf.filter((option) => validate(value, option, path).length === 0)
    return matches.length === 1 ? [] : [`${path} must match exactly one schema`]
  }
  if (rules.type && !typeMatches(value, rules.type)) return [`${path} must be ${rules.type}`]
  if (Object.hasOwn(rules, 'const') && value !== rules.const) issues.push(`${path} must equal ${JSON.stringify(rules.const)}`)
  if (rules.enum && !rules.enum.includes(value)) issues.push(`${path} must be one of ${rules.enum.join(', ')}`)
  if (rules.pattern && typeof value === 'string' && !new RegExp(rules.pattern).test(value)) issues.push(`${path} has an invalid format`)
  if (rules.format === 'date-time' && (Number.isNaN(Date.parse(value)) || !/T.*Z$/.test(value))) issues.push(`${path} must be an RFC 3339 UTC timestamp`)
  if (typeof value === 'number' && rules.minimum !== undefined && value < rules.minimum) issues.push(`${path} must be at least ${rules.minimum}`)
  if (typeof value === 'string' && rules.minLength !== undefined && value.length < rules.minLength) issues.push(`${path} is too short`)
  if (typeof value === 'string' && rules.maxLength !== undefined && value.length > rules.maxLength) issues.push(`${path} is too long`)
  if (Array.isArray(value)) {
    if (rules.uniqueItems && new Set(value.map((item) => JSON.stringify(item))).size !== value.length) issues.push(`${path} must contain unique items`)
    if (rules.items) value.forEach((item, index) => issues.push(...validate(item, rules.items, `${path}[${index}]`)))
  }
  if (typeMatches(value, 'object')) {
    for (const key of rules.required ?? []) if (!Object.hasOwn(value, key)) issues.push(`${path}.${key} is required`)
    if (rules.additionalProperties === false) {
      for (const key of Object.keys(value)) if (!Object.hasOwn(rules.properties ?? {}, key)) issues.push(`${path}.${key} is not allowed`)
    }
    for (const [key, propertyRules] of Object.entries(rules.properties ?? {})) {
      if (Object.hasOwn(value, key)) issues.push(...validate(value[key], propertyRules, `${path}.${key}`))
    }
  }
  return issues
}

function validateEvent(event) {
  const issues = validate(event, schema)
  if (event.error?.code && !errors.some((item) => item.code === event.error.code)) {
    issues.push('$.error.code is not in the stable registry')
  }
  return issues
}

function assertInvalidEvent(name, event) {
  assert.notDeepEqual(validateEvent(event), [], `${name} unexpectedly passed contract validation`)
}

function validateRegistry(rows) {
  assert.ok(Array.isArray(rows), 'error registry must be an array')
  for (const [index, row] of rows.entries()) {
    assert.ok(row && typeof row === 'object' && !Array.isArray(row), `registry row ${index} must be an object`)
    assert.deepEqual(Object.keys(row).sort(), ['class', 'code', 'retryable', 'suggestedActionKey', 'userMessageKey'], `registry row ${index} has an invalid shape`)
    assert.match(row.code, /^[A-Z]+(?:\.[A-Z_]+){2}$/)
    assert.match(row.class, /^[A-Z_]+$/)
    assert.equal(typeof row.retryable, 'boolean', `registry row ${index} retryable must be boolean`)
    assert.match(row.userMessageKey, /^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)+$/)
    assert.match(row.suggestedActionKey, /^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)+$/)
  }
}

assert.equal(schema.$schema, 'https://json-schema.org/draft/2020-12/schema')
assert.equal(schema.additionalProperties, false)
assert.ok(Array.isArray(schema.properties.eventType.enum), 'eventType must be a closed v1 enum')
for (const eventType of ['request.accepted', 'request.completed', 'request.failed']) {
  assert.ok(schema.properties.eventType.enum.includes(eventType), `${eventType} must remain in the closed v1 enum`)
}
assert.ok(schema.properties.eventType.enum.includes(fixture.eventType), 'fixture eventType must be registered')
assert.match(schema.$defs.error.properties.developerDetail.pattern, /^\^diagnostic\\\./, 'developerDetail must be a diagnostic key')
for (const field of schema.required) assert.ok(Object.hasOwn(fixture, field), 'missing ' + field)
assert.match(fixture.eventId, /^evt_/)
assert.match(fixture.correlation.traceId, /^trc_/)
assert.deepEqual(validateEvent(fixture), [], 'fixture does not satisfy the event contract')
validateRegistry(errors)
assert.deepEqual(errors.map((item) => item.code), approvedCodes, 'error registry must exactly match the approved v1 codes')
assert.equal(new Set(errors.map((item) => item.code)).size, errors.length)
const fixtureCode = errors.find((item) => item.code === fixture.error.code)
assert.ok(fixtureCode, 'fixture error code is not in the registry')
assert.deepEqual(
  {
    code: fixture.error.code,
    class: fixture.error.class,
    retryable: fixture.error.retryable,
    userMessageKey: fixture.error.userMessageKey,
    suggestedActionKey: fixture.error.suggestedActionKey
  },
  fixtureCode,
  'fixture error metadata must match the registry'
)

const invalidStatus = structuredClone(fixture)
invalidStatus.execution.status = 'UNKNOWN'
assertInvalidEvent('unknown execution status', invalidStatus)

const invalidTrace = structuredClone(fixture)
invalidTrace.correlation.traceId = 'invalid-trace'
assertInvalidEvent('malformed nested correlation field', invalidTrace)

const invalidEventType = structuredClone(fixture)
invalidEventType.eventType = 'workflow.stage.experimental'
assertInvalidEvent('unregistered event type', invalidEventType)

const unknownEvidenceField = structuredClone(fixture)
unknownEvidenceField.evidence.userInput = 'synthetic-test-value'
assertInvalidEvent('user input evidence', unknownEvidenceField)

const sensitiveEvidenceField = structuredClone(fixture)
sensitiveEvidenceField.evidence.authorization = 'synthetic-test-value'
assertInvalidEvent('authorization evidence', sensitiveEvidenceField)

const unsafeDeveloperDetail = structuredClone(fixture)
unsafeDeveloperDetail.error.developerDetail = 'private prompt and tool arguments'
assertInvalidEvent('raw developer detail', unsafeDeveloperDetail)

const unregisteredErrorCode = structuredClone(fixture)
unregisteredErrorCode.error.code = 'RENDER.FFMPEG.UNKNOWN'
assertInvalidEvent('unregistered error code', unregisteredErrorCode)

const malformedRegistry = structuredClone(errors)
malformedRegistry[0].prompt = 'synthetic-test-value'
assert.throws(() => validateRegistry(malformedRegistry), /invalid shape/)

const malformedRegistryType = structuredClone(errors)
malformedRegistryType[0].retryable = 'true'
assert.throws(() => validateRegistry(malformedRegistryType), /retryable must be boolean/)
