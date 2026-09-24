# RP-4 — culture, institutions and law

Status: RP-4 implementation, original-scope audit and full-stage local verification PASS; prepared for checkpoint commit. Layered culture, actual gift interpretation/choice, institution/law lifecycle, gradual version knowledge, sourced role transitions/recovery, private dispute/handover/review and HTTP case journey are implemented. RP-3 checkpoint `186fb68` was committed and its worktree verified clean before this phase began. Source requirements: original goal section5 (RP-4A/B/C). The acceptance audit records final verification; early recon/proposal sections below are historical.

## Final local verification and recovery point

- Full uncached Go tests and vet PASS; full race259/259 top-level tests PASS, including187 storage tests (2918.728s). No individual test skips/failures or unfinished top-level runs. CLI m1 has no tests.
- Frontend typecheck/build, configured bounded verifiers, deterministic life and local HTTP-provider browser recovery PASS. These do not claim a live external LLM result or full legacy M2 acceptance.
- Migration026 unchanged. Existing Event-backed state, idempotent commands and projection rebuild remain the recovery path; tests cover reopen/rebuild and retry. This checkpoint is a source recovery point, not permission to roll back committed world history.
- Deferred boundaries: executable culture norm is gift evaluation; executable law is speech prohibition; family is mutual chosen-family; community supplies Class/Community. No generic prose interpreter, genealogy, automatic punishment, new world-pack registry, or automatic private case inheritance. Un-enforced violations retain their original recorder; successor reviewers need explicit case delivery.
- Detailed evidence and limitations: [acceptance audit](phase-4-audit.md). Checkpoint hash and post-commit clean state are reported after commit. No deployment or push. RP-5–8 and final300-turn/30-day integration remain outstanding.

## Acceptance

- Culture contract covers values, norms, customs, taboos, rituals, status symbols, group identity, subculture, transmission, conflict and evolution. Actual scoped provenance must support World → Region → Class/Community → Organization → Family → Individual internalization; no invented family/group membership from names or prose.
- Individuals can accept, partially accept, oppose or rebel. Cultural identity influences real choices without becoming an unavoidable script or a universal modern value system.
- Institution/law lifecycle covers proposal, enactment, effective time, enforcement, violation, dispute, amendment and repeal. Reuse authority, role, norm, procedure, enforcement and evolution concepts through existing Event/ACL/clock/ledger ownership, not another world state.
- A law's existence differs from obedience; its text differs from actual enforcement. A violation cannot manufacture automatic narrative punishment. Different scoped world configurations can define different institutions.
- Direct integration evidence must show the same action evaluated differently by different groups; identification changing NPC choice; law changing lawful candidate actions; a character deliberately violating a rule and facing a sourced consequence; incomplete NPC knowledge; and enacted changes becoming known through actual propagation.
- Recovery, idempotency, authority, private knowledge, original-earned obligations and legacy replay compatibility remain mandatory. Subsequent RP-5–8 and final300-turn/30-day requirements are not absorbed into this phase or declared complete here.

## Initial reuse map

| Existing authority | Evidence inspected | Intended reuse / boundary |
| --- | --- | --- |
| Events, rule epochs, commands and scheduler | migrations001, career command/activation paths from RP-3 | Immutable proposal/version/activation provenance; no second clock or event history. |
| Principals and capabilities | migration005; `career_authority.go` | Existing scoped grants and independently sourced role projection; institution authority must not be granted merely by declaring a title. |
| Observation and Knowledge | migration007; RP-3 career announcement/hearing path | Actual recipients learn attributable statements; absent actors do not receive omniscient law/culture updates. |
| NPC input/provider/commit boundaries | core `rp_decision.go`, storage `rp_decision.go` and commit path | Filtered own-known context; model proposes, rule engine validates and commits. |
| Own disposition/relationships/memory | RP-2 Life Context and RP-3 career integration | Cultural identification needs its own sourced experience/stance, not a replacement personality or memory database. |
| Procedures with real accounting effects | Existing arrears/insolvency and career lifecycle | Reuse explicit authority/evidence/effective-time/conservation patterns; these bounded financial procedures are not already a generic legislative engine. |

Search found `Institution` labels on static inspector records in `src/data/world.ts` (employment, arrears, rent extension). They are useful vocabulary/example data, not proof of executable culture or enactment/enforcement APIs. No backend culture/law lifecycle implementation was discovered in the inspected core/migration sources; continue focused recon before choosing storage contracts.

## Architecture questions to resolve before implementation

1. Establish provenance and ownership for each cultural scope and membership. Reuse actual existing entities/cohorts/organizations; discover whether region/community/family links already have executable owners before introducing only what is missing.
2. Separate physically/technically executable actions from lawful actions. Existing `LegalActions` is an execution allowlist (`respond`, `refuse`, `silence`, `wait`, reachable `leave`), not a criminal-law model. Preserve capability/physics validation while exposing law assessments and deliberate noncompliance; do not make "can violate a law" mean "can bypass authorization".
3. Pin known versus current institutional versions. Future enactment must not apply early; an unobserved amendment must not silently replace an NPC's belief. Lawful-action assessment, choice and enforcement each need the appropriate authoritative/known inputs.
4. Enforcement must reference an actual act, applicable rule and authorized procedure. A recorded breach and a hearing/dispute are distinct from an assessed consequence; money or permissions change only through existing authorities.

## Representative slice proposal — not yet approved by evidence

Start with one real interpersonal action and two explicitly sourced cultural groups that evaluate it differently. Only actual transmission and an individual's recorded internalization may influence their next committed choice. Demonstrate rejection/partial acceptance, absent-observer ignorance, rollback and restart through real commands before expanding the culture contract. Select the action and scope/membership owner after the remaining recon; gift versus greeting are candidates, not invented product requirements.

Then extend the same evidence boundaries to one bounded institution: proposed rule → authorized enactment → future activation → observed communication → lawful/noncompliant choice → sourced violation → authorized enforcement/dispute → amendment/repeal. This sequence must not add a general-purpose prompt-to-execution interpreter or an automatic punishment script.

## Selected first slice and ownership

Scoped recon of migrations006/007, RPBackground and backend core/storage finds population lineage and real places/profiles, but no executable family/community affiliation owner. `market_quotes.region_id` is a market label, not an agent's regional membership. Background residence is not kinship; cohort origin is not cultural consent. Career organization/contract Events remain the owner of employment membership.

Introduce only Event-backed cultural scope definitions and explicit affiliations referencing existing world/entity/place/organization identities. Definitions and membership do not create people, funds, relatives or capabilities. World/region/community/organization/family are cultural scopes, not a compulsory exclusive inheritance tree: individuals can belong to overlapping groups. Organization affiliation must retain actual employment evidence when claimed as employment; family affiliation is an explicit sourced declaration, never inferred from co-residence. Individual internalization is separate from membership and requires a known transmitted version. Defining a culture cannot write another actor's stance.

The first real action is the existing `gift` SocialRP action. Two bounded group norms can favor or disapprove of the same conserved gift. Evaluation retains definition/version, transmission and stance provenance; missing knowledge produces no evaluation. Acceptance follows the norm, partial acceptance reduces its influence, opposition withholds endorsement, and explicit rebellion reverses its influence. These are advisory social evaluations, never technical action permissions or automatic punishment. Opposing/rebelling is an explicit actor decision, not a default inferred from membership. Multiple norms remain separate rather than silently selecting one universal morality.

Implementation order: typed culture/evaluation contract and boundary tests → authorized Event-backed definition/affiliation/transmission/internalization → actual gift evaluation and filtered NPC-choice integration → restart/rollback/privacy tests. The pure contract is only a dependency of the vertical slice, not evidence that culture already affects gameplay. Reuse existing command transaction invariants without recording cultural facts as career facts. No schema migration is needed for the initial pure contract.

DEFINE is resolved for this bounded slice; DESIGN for storage/authorization remains open until its concrete command path is verified. Full institution/law design and later phase gates remain open. Existing optional live-model constraint remains as reported; no production publication is authorized.

## Event-backed dependency implementation

`private_fact_command.go` extracts the existing career transaction with typed domain payloads. Career retains its IDs, policy, Event type, version and hash shape. Culture uses `RPCultureFactRecorded`, a separate namespace, and no public outbox. Both paths authorize before retry, reject payload mismatch/stale heads/unfinished RP actions, preserve chronology, and atomically record audit plus domain effects with rollback injection.

Initial Store commands: `DefineRPCulture`, `TransmitRPCulture`, `InternalizeRPCulture`. An active actor may found a new voluntary community, not declare a world/region/family/organization culture through that authority. Community definition is not membership or acceptance. Transmission must come from the author or someone who actually heard that exact definition, uses current co-location, records the listener snapshot and existing speech Knowledge, and carries the complete bounded definition. Internalization requires the actor's control and actual inclusion in the referenced listener snapshot; stance is never written automatically by the author or by hearing. A speaker is not fabricated as their own listener. More complete authored-knowledge/internalization and affiliation paths remain to implement.

Focused SQLite integration now verifies rollback, impersonation rejection, idempotency/mismatch, unread-definition relay rejection, actual hearing, no automatic stance, explicit rebellion with retained sources, private outbox exclusion and exact retry across database reopen. The first fixture failed because no two NPCs were initially co-located; it now advances the real day schedule to noon before transmission. This is a persistence dependency, not a completed culture-to-life acceptance: actual gift evaluation storage, filtered NPC context/choice, affiliations/all layers, HTTP surface and institution lifecycle remain open.

Validation: combined core/storage `TestRPCulture|TestCareer` PASS (0.003s/32.692s), focused culture persistence race PASS (4.094s), core/storage vet and diff check PASS. Full unfiltered repository regression and full RP-4 gate not run for this increment.

## Representative gift/choice slice

`rp_culture_life.go` reads only the actor's internalizations, joining their exact scoped definition Events. A social action is evaluated against the latest stance **before that action's immutable Event sequence**; a later stance cannot rewrite old experience. This is a derived private view of committed facts, not a new mutable relationship/personality authority. Unknown cultures have no influence. Each norm/source stays in `Life.CultureExperiences`; the bounded relationship interpretation averages matching norms, with no cultural evaluation retaining the legacy gift interpretation. Positive/negative/neutral evaluations affect affinity/trust/tension but never undo a conserved gift or manufacture punishment.

Provider input includes only the NPC's own experiences. Giver Knowledge, common action payload and participant outbox do not contain the recipient's private stance/evaluation. Deterministic choice honors existing financial/work/commitment priorities first, then may refuse conversation after a negatively evaluated gift. This is an executable validated proposal, not a law prohibition or forced compliance.

Real SQLite integration has two worlds with opposite gift norms: same one-unit conserved gift leads Cai to respond versus refuse, and both choices commit through `CommitRPDecision`. Explicit rebellion subsequently reverses the next gift evaluation/committed choice while keeping the earlier experience unchanged. Tests also check private Knowledge exclusion, exact decision retry after reopen, projection rebuild and compare. Initial Ada fixture could not isolate this effect because Ada's next work appointment already caused refusal; changed to Cai without weakening work constraints. Focused culture/social/decision/NPC regression PASS(core0.002s/storage3.470s). Culture race and vet tracked in progress.

Culture slice race PASS13.577s; core/storage vet and diff check PASS. Initial authenticated HTTP commands and authored self-internalization are now implemented; see [culture-api.md](culture-api.md). Authored knowledge references the definition Event rather than inventing self-hearing. Actor control and ownership are enforced for new commands and retries. Focused core/storage/HTTP culture tests PASS(0.017s/1.073s/0.332s); HTTP Career regression PASS3.481s after shared response-handler extraction. Remaining full scope: sourced affiliations and all requested layers; culture evolution/conflict beyond independent gift norms; complete law/institution lifecycle and agent integration; broad replay/legacy/privacy/long-run stage gate. Do not mark RP-4 complete from this slice.

## Voluntary community membership increment

`AffiliateRPCulture` and authenticated `/api/v1/culture/affiliate` now record self-controlled join/leave/rejoin. Joining requires actual authorship or hearing of the referenced community version; leaving requires existing own active membership. No author may force another member, no stance is inferred from membership, and opposing a norm does not expel a person. Derived own `Life.CultureAffiliations` carries the latest sourced status without publishing private membership to peers. Old exact join retries cannot resurrect a departed member. This is community membership only, not all-layer authority or a family/region model.

Actual SQLite tests cover unseen definition, forced-membership rejection, atomic rollback, joining without a stance, opposition while remaining a member, leaving without erasing stance, reopen/historical retry/rebuild, and rejoin. HTTP tests cover creator versus member control. Focused core/storage/HTTP culture suite PASS(0.002s/1.397s/0.325s). Layer definitions, region/place assignments, sourced family/organization affiliations, evolution and all institution/law requirements still remain; stage not complete.

## Organization layer increment

Organization culture now references an actual Career organization Event and requires both author control and an active scoped manager grant, including retries. Definition grants no new permission. Joining requires source-backed effective own employment plus authored/heard culture knowledge; a future onboarding contract is insufficient. Affiliation retains the contract and eligibility Event at joining. This is cultural identity, not a second employment roster; ending employment must not automatically rewrite identity/history.

Real employment integration verifies employee/manager distinction, revoked-manager retry rejection (test-only ACL revocation/restoration), onboarding rejection, actual start/schedule followed by news and voluntary join, retained contract provenance, no automatic stance/role grant and rebuild. Focused organization test PASS0.350s. Remaining layers: world, region/place and family ownership/provenance, plus full evolution/institution scope. No full-stage gate claim.

## Family layer increment

Added explicit co-located chosen-family proposal and invitee-controlled confirmation Events, with authenticated endpoints. Only a confirmed participant can define family-scoped culture or claim its cultural affiliation, retaining the acceptance Event as eligibility/scope evidence. Declaration, cultural affiliation and internalization stay separate. No inference of biological descent/marriage/guardianship or automatic shared norms is made. Initial family source has two consenting participants; expansion/dissolution and genealogy are not implemented.

Actual SQLite test covers unconfirmed/rolled-back proposal rejection, proposer unable to accept for invitee, confirmed scope source, outsider rejection despite co-location/hearing, own affiliation without automatic stance, explicit rebellion and reopen/rebuild. Focused family test PASS0.309s; family race PASS4.499s; core/storage/HTTP vet PASS. HTTP route test covers two-principal proposal/acceptance and denied impersonation. World/region scope, evolution/conflict and full law/institution acceptance remain open.

## World/region scope increment

Explicit territory configuration requires active creator identity plus the original branch's genesis-backed Cohort construction grant (same source, branch and sequence1), not arbitrary creator identity or an unrelated later grant. It records builder/place source Events and atomically gives a real scoped agent only the dedicated existing-ACL capability `world.culture.define`. Legacy bootstrap/hash inputs remain untouched. World scope references the actual instance; regions reference existing active scoped places. Dedicated scope authorization is rechecked before definition retries.

World/region cultures use the same actual speech/internalization path; definition does not inform everyone. Regional affiliation additionally requires real presence and retains its source Event/place; membership is cultural identity, not ongoing geographical location. Real test verifies unauthorized setup/steward, nonexistent place, rollback without grant leakage, offsite join rejection, actual scheduled presence, world scope source, no auto-stance/omniscient relay and rebuild retaining presence lineage. Focused territory test PASS0.309s. Broad culture regression/race status tracked in progress.

Required follow-up before any full-stage pass: explicit territory HTTP tests and loss/corruption repair for dedicated grants. Current rebuild test preserves extant grants; it does not prove reconstruction after grant deletion. Culture evolution/subculture/conflict and entire institutional lifecycle remain open.

## Territory grant repair and HTTP follow-up

The previously open grant-loss gap is now covered by `rp_culture_projection.go`, integrated into existing CompareProjections/RebuildProjections. Expected dedicated grants derive only from committed territory Events. Comparison covers missing/extra grant IDs, principal, instance/branch, subject, capability, status, source, fields and amount limit, including changed capability on a known grant ID. Repair removes exact diagnosed projection rows and restores only authoritative grants within the existing rebuild transaction. Unrelated ACL grants are outside this domain. Authorization additionally joins the installed grant to its matching territory source to reject forged authority before repair.

Actual temporary-DB corruption tests delete grants, alter identity/subject/fields/status/amount, change capability, and insert an unsourced grant. They verify detected differences, repaired equality, unchanged world head, reopen and real subsequent steward definition. Scope grant test PASS0.313s. Territory HTTP creator/steward distinction and exact retry PASS0.357s in combined HTTP fixture. Wider culture/replay/projection regression and race tracked in progress. Scope role revocation/reassignment needs a future sourced command; unsourced manual ACL editing is not a durable world action. Full culture evolution/law scope remains open.

## Sourced territorial authority lifecycle

Added builder-authorized revoke/appoint command and HTTP endpoint. Immutable `territory_authority` Events retain scope/place identity and builder provenance, update the same dedicated grant, and drive ordered projection replay. Runtime authorization rejects stale installation grants when a newer authority Event exists. New steward authorization and old knowledge are separate: replaced authors may relay old known culture but cannot define new scope culture or use exact old definition retries to bypass revocation.

Real integration verifies unauthorized steward change, rollback without loss of old permission, durable revoke, forged old projection rejected before repair, reopen/rebuild retaining revoked state, appointment/new steward use, old steward denial, retained old knowledge and final projection equality. Focused lifecycle PASS0.279s. HTTP test covers builder-only revoke. Full stage remains open for culture evolution/subculture/conflict and institution/law lifecycle.

## Cultural evolution, subculture and conflicting identification

Definitions now support explicit previous-version CAS and pinned parent-definition evidence. Revision retains culture/scope identity and current authority and requires the author to know the current version; community ownership cannot be hijacked by a listener. Subculture parents must be actual known versions, matched to declared parent IDs; no self/duplicate parent or invented lineage. Every version remains immutable and individually transmissible. Receivers retain their old stance until explicit internalization of a newly heard version; merely hearing is not acceptance.

Actual gift tests prove published-but-unheard and heard-but-not-adopted versions both preserve old positive evaluation; adopted new version changes subsequent evaluation to negative. An explicitly adopted positive subculture alongside negative parent creates retained private conflict evidence, not discarded provenance. Old experience retains its old version after reopen/rebuild. Focused evolution PASS0.390s, race PASS5.990s; broader culture/social regression PASS(core0.002s/storage4.590s/HTTP0.704s), core/storage/HTTP vet PASS. HTTP revision-specific check is tracked in progress.

Next RP-4 focus: bounded institution/law lifecycle, lawful versus executable actions, deliberate violation and sourced enforcement/dispute, followed by a complete original-requirement audit and broad stage regression. Culture has a representative gift norm, not an unconstrained prose-to-execution interpreter or proof that all original RP-4 criteria are complete.

Institution BUILD dependency now implements actual scope/funded-treasury installation with distinct roles, actor proposals, and authorized strictly-future enactment under a separate Event domain. See [institutions.md](institutions.md) for design, implemented boundary and missing lifecycle/agent integration. No automatic Knowledge, fine or enforcement is created by these commands. Existing technical `LegalActions` is unchanged.
