# Institutional commands over HTTP

All routes below are authenticated POST commands and return the existing `InstitutionRecord` envelope. They reuse `handleBoundCommand`: principal identity comes from authentication; a conflicting `binding.principal_id` is rejected. Body decoding, method/body bounds and domain error mapping follow existing Career/Culture conventions. Binding also includes instance, branch, expected head and idempotency key. Actor control, current dedicated role, actual knowledge and procedural source checks still run in storage, including authorization before exact retries.

| Route | Request type | Storage operation |
|---|---|---|
| `/api/v1/institutions/define` | `InstitutionDefinitionRequest` | `DefineRPInstitution` |
| `/api/v1/institutions/authority` | `InstitutionAuthorityRequest` | `ChangeRPInstitutionAuthority` |
| `/api/v1/laws/propose` | `LawProposalRequest` | `ProposeRPLaw` |
| `/api/v1/laws/enact` | `LawEnactmentRequest` | `EnactRPLaw` |
| `/api/v1/laws/announce` | `LawAnnouncementRequest` | `AnnounceRPLaw` |
| `/api/v1/laws/violations/record` | `LawViolationRequest` | `RecordRPLawViolation` |
| `/api/v1/laws/enforce` | `LawEnforcementRequest` | `EnforceRPLaw` |
| `/api/v1/laws/dispute` | `LawDisputeRequest` | `DisputeRPLaw` |
| `/api/v1/laws/disputes/forward` | `LawDisputeForwardRequest` | `ForwardRPLawDispute` |
| `/api/v1/laws/review` | `LawReviewRequest` | `ReviewRPLawDispute` |

Amendment/repeal uses proposal and enactment with `previous_enactment_event_id` and `repealed`; no endpoint bypasses version/time rules. Role authority uses explicit builder-authorized `revoke`/`appoint`. Forwarding resubmits the affected actor's existing private dispute to the active reviewer, not a global case-access capability. No public list-all institution case or actor-belief endpoint is introduced.

HTTP evidence: all ten routes reject unauthenticated and forged-principal requests. Real DB/server test covers funded organization/territory → institution → controlled proposal → role-authorized enactment → restart/exact retry → known-speaker announcement → builder revocation → denial of revoked enactment retry (PASS0.287s). Culture HTTP regression PASS0.341s. Complete HTTP violation/fine/dispute/review/forward lifecycle and a dedicated authorized own-case query remain to be exercised; storage tests do not substitute for those transport tests.

## Own-case read

`POST /api/v1/laws/cases/own` accepts `principal_id` (optional, bound to authentication), `instance_id`, `branch_id`, `actor_id`. No mutation binding, expected head or idempotency key is required. It authorizes control of the requested actor and reads the existing Event-derived last16 own case receipts in one transaction, returning an array (empty `[]` when none). Institution office alone grants no access to another actor's receipt view. Responses omit treasury account details and other defendants.

HTTP tests verify unauthenticated denial, forged identity denial, wrong actor denial and stable empty-array output. The real storage fine/dispute test also calls the exported reader to verify a populated receipt and reject the enforcer reading the defendant's own cases. Focused storage PASS2.071s and HTTP PASS0.287s plus package vet. The complete HTTP populated-case journey remains required and is not implied by these split tests.

## Complete HTTP case journey

The extended real server/database test (PASS0.462s) now covers the populated journey, superseding the missing-evidence notes above. After scheduled noon co-location and a real law announcement, a player opens a session and commits an actual settled speech turn through HTTP. The witnessed act is recorded as a violation, fined2, read only by the affected player, and disputed by that player. A newly appointed reviewer cannot review before explicit forwarding; after forwarding, the reviewer reverses the actual fine with receipt provenance. Server/store reopen and exact review retry preserve the same Event and final own-case status/refunded2. The test also rejects non-enforcer violation recording, enforcer reading another actor's own cases and nondefendant filing. Existing storage tests supply detailed journal conservation/rollback assertions; this HTTP test proves the integrated transport journey, not a browser UI.
