/* Verifies M2 T09, bounded Agent and unattended spatial evidence without rewriting historical M0 status. */
const fs = require('fs')
const path = require('path')

const root = path.resolve(__dirname, '..')
const evidence = JSON.parse(fs.readFileSync(path.join(root, 'docs/m2/test-evidence.json'), 'utf8'))
const m0 = JSON.parse(fs.readFileSync(path.join(root, 'docs/m0/test-vectors.json'), 'utf8'))
const tests = [
  'backend/internal/storage/m2_bootstrap_test.go',
  'backend/internal/storage/cohort_test.go',
  'backend/internal/storage/agent_test.go',
  'backend/internal/storage/agent_routine_test.go',
  'backend/internal/storage/agent_driver_test.go',
  'backend/internal/transport/httpapi/server_test.go',
  'backend/cmd/corerp-m2/main_test.go',
].map((file) => fs.readFileSync(path.join(root, file), 'utf8')).join('\n')
const failures = []
const check = (condition, message) => { if (!condition) failures.push(message) }

check(evidence.schema_version === 'm2-unattended-agent-evidence-2026-09-23', 'unexpected M2 evidence schema version')
check(evidence.status === 'passed_t09_agent_and_30day_spatial_slices_full_m2_deferred', 'M2 status must preserve full-M2 deferral')
check(Array.isArray(evidence.deferred) && evidence.deferred.length >= 5, 'broader M2 deferrals must remain explicit')
const vector = m0.vectors.find((candidate) => candidate.id === 'T09')
check(vector?.stage === 'M2' && vector?.status === 'not_run', 'historical M0 T09 definition must remain unchanged')
for (const test of evidence.tests) {
  check(tests.includes(`func ${test}(`), `T09 references missing Go test ${test}`)
}
check(typeof evidence.evidence === 'string' && evidence.evidence.length > 100, 'T09 lacks complete evidence text')

if (failures.length) {
  console.error(`M2 T09 evidence verification failed (${failures.length})`)
  for (const failure of failures) console.error(`- ${failure}`)
  process.exit(1)
}
console.log(`M2 bounded evidence verification passed: ${evidence.tests.length} executable tests; full M2 remains deferred`)
