# F9 local phase report — real author packs and lifecycle

Status: **local acceptance PASS**. Baseline is F8 checkpoint
`271d892b7b2195acef49f23d0ebe95c5f55b1504`. The containing scoped commit is the
F9 checkpoint; verify its clean worktree before entering F10. No push or release.

## Implemented and demonstrated

- **Two real author packs implemented as pure data**:
  - **Retail Career Content Pack** (`docs/f9/retail-career/`): manifest and
    content declaring job positions (`position_retail_floor`), qualification
    credentials (`retail_customer_service`), work schedule (08:00–17:00),
    wage references (16 credits), training programs (`program_retail_customer_service`),
    and career progression (`next_grade: "senior"`).
  - **Life Journal Narrative Pack** (`docs/f9/life-journal/`): manifest and
    narrative style declaring first-person past-tense presentation, detailed
    verbosity, and long narrative density (`narrative_density: "long"`).
- **Generic Pack Platform expansion**:
  - Added bounded, declarative `content` package kind and typed
    `StudioRetailCareerCatalog` contract to `core.StudioPackageBundle`.
  - Extended `ActivateStudioPackages` and `StudioPackageLock` to support
    generic content package pins (`Content []StudioPackagePin`) alongside
    system and narrative bundles.
  - Zero package-ID-specific host hacks or branches in Go or Vue.
- **Retail Career System / Content integration without parallel authority**:
  - `TestF9RetailCareerRealDomainIntegration` proves the end-to-end path:
    operator registers the pack's declared training program; candidate Ada
    enrolls, advances time, and completes the exercise; organization coop employer
    completes training and issues the credential; manager Bo posts the position
    with the pack's wage reference and required qualification; candidate applies,
    interviews, is evaluated, receives offer, and accepts.
  - Real employment contract is formed with shift appointments (08:00–17:00) and
    salient RP life memories derived in `RPDecisionInput.Life.Employment`.
  - Career progression: candidate receives promotion offer to `next_grade` ("senior")
    and accepts.
  - Existing Career and payroll systems retain sole authority over shifts and wage
    disbursements; the Pack provides only declarative reference data.
- **Long-form Life Journal Narrative Pack presentation and regeneration**:
  - `TestF9LifeJournalInstalledActivatedAndRegeneratedWithoutWorldMutation` proves
    the authored style is resolved in active player sessions and produces
    rich first-person narrative for committed turns.
  - Alternative presentation rendering with different POV and density changes
    expression only: Event IDs, branch sequence, world facts, and financial balances
    remain strictly identical.
- **Pack Platform Lifecycle Acceptance**:
  - `TestF9RetailAuthorBundleValidatedAsReferences`: validates manifest, version,
    engine API, schema hash, content hash, and rejects illegal capabilities or executable callbacks.
  - `TestF9RetailContentInstallationActivationPinsAndRestart`: verifies the distinction
    between installation (saving immutable bundle) and activation (pinning lock in rule epoch),
    historical event package inspection, and SQLite reopen/Compare.
  - `TestF9PlatformUpgradeFailurePreservesActiveLock`: verifies that illegal or failed
    activation attempts leave the prior active epoch and pinned lock intact.
  - `TestF9PlatformDisableUnloadWithHistoricalReferences`: verifies that closing an active
    epoch or unloading packages preserves historical event references and inspection provenance.
  - `TestF9PlatformDatasetSwapWithoutHostChanges`: verifies dataset swap with second
    independently authored content ("Bookstore curator") and narrative ("Chronicle")
    bundles running cleanly without modifying Go or Vue code, plus duplicate install idempotency.

## Verification

### Original F9 requirement mapping

| Requirement | Current evidence |
| --- | --- |
| Retail Career Example: job, qualification, schedule, wage ref, training, progression, org integration, RP memory | `TestF9RetailCareerRealDomainIntegration`: complete flow through Education, Career, shifts, promotion, and RP context; existing Career/Wage authority preserved |
| Long-form Life Journal Example: style, density, POV, long target, regenerate, same world result | `TestF9LifeJournalInstalledActivatedAndRegeneratedWithoutWorldMutation`: first-person, long density, read-only variant rendering, identical Event IDs and branch head |
| Manifest / dependency / version / hash validation | `TestF9RetailAuthorBundleValidatedAsReferences` |
| Install / activate distinction | `TestF9RetailContentInstallationActivationPinsAndRestart` |
| Illegal capability reject | `TestF9RetailAuthorBundleValidatedAsReferences/illegal_capability` |
| Duplicate install idempotency across restart | `TestF9PlatformDatasetSwapWithoutHostChanges` |
| Failed upgrade preserves prior active state | `TestF9PlatformUpgradeFailurePreservesActiveLock` |
| Disable / unload with historical references | `TestF9PlatformDisableUnloadWithHistoricalReferences`: historical events accurately inspect original pinned locks across epoch transitions |
| No package-ID-specific host hack | Pure generic content and style schemas throughout core and storage |
| Dataset swap without Go/Vue host changes | `TestF9PlatformDatasetSwapWithoutHostChanges`: second valid dataset set installs, validates, and activates with zero host edits |

### Test suite results

- **F9 focused storage & core tests**: PASS (4.537s storage, core cached).
- **Targeted race detection**: PASS (`core` 1.017s, `storage` 107.280s).
- **Fast packages (`core`, `decision`, `narrative`, `transport/httpapi`)**: PASS (45.071s).
- **MCP client integration**: PASS 4/4 (8.627s).
- **Static checks**: `go vet -p=1 ./...` PASS (0 errors); `git diff --check` PASS (clean).

## Documented limitations

1. **Declarative references only**: Pack definitions provide typed references for existing
   domain managers (e.g. training prompts, shift hours, wage references). They cannot
   execute code, run raw SQL, or bypass capability authorization.
2. **Supported scheduler environments**: Authoritative Career employment and shift execution
   remain anchored to the supported M2 scheduler world; Studio new-world generic scheduler
   generalization remains future work.
3. **Deterministic presentation**: Narrative renderer validates structural long density
   and style adherence against committed facts; it does not claim live LLM literary prose quality.
