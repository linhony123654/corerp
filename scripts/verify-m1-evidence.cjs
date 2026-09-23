/* Verifies the M1 evidence crosswalk without mutating the historical M0 vectors. */
const fs = require('fs')
const path = require('path')

const root = path.resolve(__dirname, '..')
const evidence = JSON.parse(fs.readFileSync(path.join(root, 'docs/m1/test-evidence.json'), 'utf8'))
const m0 = JSON.parse(fs.readFileSync(path.join(root, 'docs/m0/test-vectors.json'), 'utf8'))
const tests = [
  'backend/internal/storage/store_test.go',
  'backend/internal/storage/events_test.go',
  'backend/internal/transport/httpapi/server_test.go',
].map((file) => fs.readFileSync(path.join(root, file), 'utf8')).join('\n')
const failures = []

function check(condition, message) {
  if (!condition) failures.push(message)
}

check(evidence.schema_version === 'm1-evidence-2026-09-22', 'unexpected M1 evidence schema version')
check(evidence.vectors.length === 12, 'M1 evidence must map T01-T12')
check(m0.status === 'defined_not_run', 'historical M0 vector status must remain unchanged')
check(m0.vectors.every((vector) => vector.status === 'not_run'), 'historical M0 vectors must remain not_run')

for (const [index, vector] of evidence.vectors.entries()) {
  const expectedID = `T${String(index + 1).padStart(2, '0')}`
  check(vector.id === expectedID, `expected ${expectedID}, found ${vector.id}`)
  check(typeof vector.evidence === 'string' && vector.evidence.length > 20, `${vector.id} lacks evidence text`)
  for (const test of vector.tests) {
    check(tests.includes(`func ${test}(`), `${vector.id} references missing Go test ${test}`)
  }
}

for (const id of ['T01', 'T02', 'T03', 'T04', 'T05', 'T06', 'T07', 'T08']) {
  check(evidence.vectors.find((vector) => vector.id === id)?.status === 'passed', `${id} must be passed for M1`)
}
check(evidence.vectors.find((vector) => vector.id === 'T10')?.status === 'passed_m1_backend_transport_m4_ui_deferred', 'T10 backend transport must pass while M4 UI remains explicit')

if (failures.length) {
  console.error(`M1 evidence verification failed (${failures.length})`)
  for (const failure of failures) console.error(`- ${failure}`)
  process.exit(1)
}

console.log('M1 evidence verification passed: T01-T08 and T10 backend transport passed; M4 UI remains deferred')
