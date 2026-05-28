#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="${ROOT_DIR}/data/proof-audits/${STAMP}"
SUMMARY_FILE="${OUT_DIR}/SUMMARY.md"

mkdir -p "${OUT_DIR}"

GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"

TEST_LABELS=(
  "api-world-first-contract"
  "api-author-replay-contract"
  "api-proof-archive-contract"
  "runtime-population-lifecycle-contract"
  "runtime-200-sample-matrix"
  "runtime-200-real-world-matrix"
  "api-200-sample-matrix"
  "api-200-real-world-matrix"
)
TEST_PACKAGES=(
  "./internal/api"
  "./internal/api"
  "./internal/api"
  "./internal/runtime"
  "./internal/runtime"
  "./internal/runtime"
  "./internal/api"
  "./internal/api"
)
TEST_PATTERNS=(
  "^(TestAPIContractCanonicalSchemasExcludeLegacyFocusMirrors|TestStateIncludesInstanceMetadata|TestRuntimeAuditAggregatesAuthoringEvidence|TestInstancesEndpointUsesParticipantsAsSceneTruth|TestInstanceCreateEndpoint|TestInstanceCreateEndpointIgnoresActiveCharacterFallback|TestInstanceStatusEndpoint|TestCharactersRouteUsesParticipantsView|TestCharactersRouteDoesNotFallbackToLoadedCharacters|TestMemoryRoutePrefersFocusCharacterOverLegacyCharacter|TestPendingFactsRoutePrefersFocusCharacterOverLegacyCharacter|TestTraceRoute|TestTraceRoutesNormalizeLegacyCharacterFields|TestCheckpointAndPresetRoutes|TestNPCActionsRouteUsesFocusCharacterWithoutTopLevelCharacterMirror)$"
  "^(TestExperimentReportReplayCreatesReplayBranches|TestExperimentReportReplayBatchFiltersByWorld|TestExperimentReportReplayBatchRealRuntimeRoundTrip|TestExperimentReportReplayAdvanceTicksReplayBranches|TestRuntimeAuditAggregatesAuthoringEvidence)$"
  "^TestProofAuditsRouteListsLatestAuditArtifacts$"
  "^(TestReconcilePopulationPromotesBackgroundNPC|TestReconcilePopulationDemotesStalePromotedNPC|TestAutonomousSimulationPromotesScenePopulationAcrossLongWindow|TestPopulationInsightsIncludesPromotionReason)$"
  "^TestWorldOutcomeSampleMatrixAcrossTwoHundredTicks$"
  "^TestRealWorldDirectorySampleMatrixAcrossTwoHundredTicks$"
  "^TestAPIWorldOutcomeSampleMatrixAcrossTwoHundredTicks$"
  "^TestAPIRealWorldDirectorySampleMatrixAcrossTwoHundredTicks$"
)

{
  echo "# World Proof Audit"
  echo
  echo "- Created At (UTC): ${STAMP}"
  echo "- Repo: ${ROOT_DIR}"
  echo "- Git Commit: $(git -C "${ROOT_DIR}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
  echo "- Git Dirty: $(test -n "$(git -C "${ROOT_DIR}" status --short 2>/dev/null)" && echo yes || echo no)"
  echo "- Go Binary: ${GO_BIN}"
  echo
  echo "## Scope"
  echo
  echo "- api world-first contract checks"
  echo "- api author replay contract checks"
  echo "- api proof archive contract checks"
  echo "- runtime population lifecycle contract checks"
  echo "- runtime 200 tick sample matrix"
  echo "- runtime 200 tick real world matrix"
  echo "- api 200 tick sample matrix"
  echo "- api 200 tick real world matrix"
  echo
  echo "## Results"
  echo
} > "${SUMMARY_FILE}"

overall_status=0

for i in "${!TEST_LABELS[@]}"; do
  label="${TEST_LABELS[$i]}"
  pkg="${TEST_PACKAGES[$i]}"
  pattern="${TEST_PATTERNS[$i]}"
  log_file="${OUT_DIR}/${label}.log"
  started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  start_epoch="$(date +%s)"

  echo "==> ${label}"
  if (
    cd "${ROOT_DIR}" &&
    "${GO_BIN}" test -count=1 -run "${pattern}" "${pkg}"
  ) > "${log_file}" 2>&1; then
    status="PASS"
  else
    status="FAIL"
    overall_status=1
  fi

  end_epoch="$(date +%s)"
  duration="$((end_epoch - start_epoch))"

  {
    echo "### ${label}"
    echo
    echo "- Status: ${status}"
    echo "- Package: \`${pkg}\`"
    echo "- Pattern: \`${pattern}\`"
    echo "- Started At (UTC): ${started_at}"
    echo "- Duration Seconds: ${duration}"
    echo "- Log: \`${log_file#${ROOT_DIR}/}\`"
    echo
  } >> "${SUMMARY_FILE}"

  if [[ "${status}" == "FAIL" ]]; then
    echo "FAILED: ${label}. See ${log_file}" >&2
  fi
done

{
  echo "## Final"
  echo
  if [[ "${overall_status}" -eq 0 ]]; then
    echo "- Overall: PASS"
  else
    echo "- Overall: FAIL"
  fi
  echo "- Output Directory: \`${OUT_DIR#${ROOT_DIR}/}\`"
} >> "${SUMMARY_FILE}"

echo "Proof audit written to ${OUT_DIR}"
exit "${overall_status}"
