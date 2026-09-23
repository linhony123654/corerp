/* CoreRP M0 contract/static-fixture verification. No backend behavior is implied. */
const crypto = require('crypto')
const fs = require('fs')
const path = require('path')

const root = path.resolve(__dirname, '..')
const failures = []
let checks = 0

function check(condition, message) {
  checks += 1
  if (!condition) failures.push(message)
}

function readJson(relative) {
  return JSON.parse(fs.readFileSync(path.join(root, relative), 'utf8'))
}

function canonical(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`
  return `{${Object.keys(value)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${canonical(value[key])}`)
    .join(',')}}`
}

function hashJson(value) {
  return `sha256:${crypto.createHash('sha256').update(canonical(value)).digest('hex')}`
}

const schema = readJson('docs/m0/core-contract.schema.json')
const vectors = readJson('docs/m0/test-vectors.json')
const schemaHash = hashJson(schema)
const examples = ['qingxuan', 'metro'].map((name) => ({
  name,
  manifest: readJson(`docs/m0/examples/${name}/manifest.json`),
  world: readJson(`docs/m0/examples/${name}/world.json`)
}))

check(schema.$schema === 'https://json-schema.org/draft/2020-12/schema', 'JSON Schema draft must be 2020-12')
for (const def of ['command', 'event', 'eventBatch', 'packageManifest', 'principal', 'capability', 'scope']) {
  check(Boolean(schema.$defs?.[def]), `missing JSON Schema definition: ${def}`)
}

check(vectors.status === 'defined_not_run', 'test vectors must not claim runtime execution')
check(vectors.vectors.length === 12, 'test vector set must contain T01-T12')
check(
  vectors.vectors.every((v, index) => v.id === `T${String(index + 1).padStart(2, '0')}`),
  'test vector IDs must be contiguous T01-T12'
)
check(vectors.vectors.every((v) => v.status === 'not_run'), 'all M0 vectors must remain not_run before backend execution')

const requiredWorldKeys = [
  'schema_version',
  'world_definition',
  'currencies',
  'entities',
  'accounts',
  'skus',
  'inventory',
  'employment_contracts',
  'rent_contracts',
  'scheduler_seed',
  'extensions'
]

for (const { name, manifest, world } of examples) {
  check(manifest.kind === 'world', `${name}: manifest kind must be world`)
  check(manifest.schema_hash === schemaHash, `${name}: schema_hash does not match canonical schema bytes`)
  check(manifest.content_hash === hashJson(world), `${name}: content_hash does not match canonical world.json`)
  check(manifest.content_files.length === 1 && manifest.content_files[0] === 'world.json', `${name}: unexpected content file set`)
  check(requiredWorldKeys.every((key) => Object.hasOwn(world, key)), `${name}: world definition shape is incomplete`)
  check(world.world_definition.id === manifest.id, `${name}: world definition ID differs from manifest ID`)
  check(world.entities.length === 6, `${name}: vertical slice must contain organization, 3 workers, landlord, and store`)
  check(world.employment_contracts.length === 3, `${name}: vertical slice must contain 3 employment contracts`)
  check(world.accounts.every((a) => Number.isSafeInteger(a.opening_balance_minor)), `${name}: account amounts must be safe integers`)
  check(world.inventory.every((i) => Number.isSafeInteger(i.opening_quantity_minor)), `${name}: inventory quantities must be safe integers`)
}

check(
  JSON.stringify(Object.keys(examples[0].world).sort()) === JSON.stringify(Object.keys(examples[1].world).sort()),
  'both world packs must use the same core top-level shape'
)

const rfc = fs.readFileSync(path.join(root, 'docs/m0/core-contract-rfc.md'), 'utf8')
for (const phrase of [
  'Event Batch',
  '[start_sequence, end_sequence)',
  '(world_time, phase_id, declared_priority, scheduler_item_id)',
  'Principal + Capability + Scope',
  'draft -> posted',
  'RFC 8785'
]) {
  check(rfc.includes(phrase), `RFC is missing required contract phrase: ${phrase}`)
}

const worldSource = fs.readFileSync(path.join(root, 'src/data/world.ts'), 'utf8')
const typeSource = fs.readFileSync(path.join(root, 'src/types.ts'), 'utf8')
check(!typeSource.includes('sequence_range'), 'demo RuleEpoch must not use ambiguous sequence_range')
check(!typeSource.includes('由事件派生或事务内权威'), 'demo snapshot must not retain the old dual-authority wording')
check(typeSource.includes('record_order: number'), 'demo records must separate record_order')
check(typeSource.includes('event_sequence: number | null'), 'demo records must expose nullable event_sequence')
check(worldSource.includes("event_type: 'LawActivationEvent'"), 'activation visualization fixture is missing')
check((worldSource.match(/schema_hash: 'sha256:[0-9a-f]{64}'/g) ?? []).length === 5, 'demo pack schema hashes must use canonical hash format')
check((worldSource.match(/content_hash: 'sha256:[0-9a-f]{64}'/g) ?? []).length === 5, 'demo pack content hashes must use canonical hash format')

const chunks = worldSource.split(/(?=\n  \{\n    record_id: C\.rec\()/).slice(1)
const records = chunks.map((chunk) => ({
  id: Number(chunk.match(/record_id: C\.rec\((\d+)\)/)?.[1]),
  type: chunk.match(/record_type: '([^']+)'/)?.[1],
  order: Number(chunk.match(/\n    record_order: (\d+)/)?.[1]),
  eventSequence: chunk.match(/\n    event_sequence: (null|\d+)/)?.[1],
  eventType: chunk.match(/event_type: '([^']+)'/)?.[1],
  epoch: Number(chunk.match(/\n    rule_epoch: (\d+)/)?.[1])
}))
check(records.length === 32, 'demo must retain 32 audit/event records')
check(records.every((r, index) => r.order === index + 1), 'record_order must be contiguous 1..32')
check(records.every((r) => r.eventSequence !== undefined), 'every demo record must declare event_sequence or null')
const events = records.filter((r) => r.type === 'event')
check(
  events.every((r, index) => Number(r.eventSequence) === index + 1),
  'authoritative event_sequence must be contiguous and must ignore audit records'
)
check(
  records.filter((r) => r.type !== 'event').every((r) => r.eventSequence === 'null'),
  'audit and diagnostic records must not advance event_sequence'
)
const activation = events.find((r) => r.eventType === 'LawActivationEvent')
check(activation?.eventSequence === '12' && activation?.epoch === 0, 'activation event must be event 12 in the old epoch')

if (failures.length) {
  console.error(`M0 verification failed: ${failures.length}/${checks} checks`) 
  for (const failure of failures) console.error(`- ${failure}`)
  process.exit(1)
}

console.log(`M0 verification passed: ${checks} checks`)
