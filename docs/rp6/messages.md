# RP6 phone transaction notices

The on-demand **手机** entry shows dated own work notices and response receipts. It is an in-world presentation of existing Career facts, **not** a new SMS/private-chat transport. Sending, replying, read receipts and fabricated sender avatars are not implemented or claimed. In-person announcements and ambient speech remain in their actual hearing/history paths, never converted into phone messages.

## Read contract

`POST /api/v1/rp/messages/read` accepts `session_id` and optional nonnegative `before_sequence` (0 means latest). Principal comes from authentication. Every page validates active session, live control and world/entity binding, and reads current clock/head in the same transaction. It appends no Events, advances no clock and changes no world-action bookmark or knowledge.

Select only this controlled candidate's `RPCareerFactRecorded` in the actual instance/branch, with an explicit kind/status allowlist:

- interview invitations and responded-answer receipts;
- offered/accepted/declined/expired employment offers when such a fact exists;
- own accepted employment/change notices;
- approved/rejected leave decisions;
- offered/accepted/declined position adjustments;
- offered/accepted/declined/cancelled overtime agreements.

The DTO contains only message ID/sequence/world time, kind, localized title and a selected notice/receipt body. It never exposes whole CareerFact, evaluation/performance/position-assessment fields, account IDs, capability lists or private advisory reasoning. No notification is manufactured merely because another event mentions the candidate. Records retain historical status; an old offer is not proof that an offer remains actionable today. Current effective employment stays in the work view.

Pagination orders immutable event sequence descending, fetches51, returns at most50 and gives the last returned sequence only if another page exists. Next page uses strict `<` and retains stable order without duplicates. Refresh starts from newest; the whole multi-page listing is not a frozen snapshot across newly appended events. UI deduplicates by immutable ID and preserves labeled old content after read failure. Body uses escaped Vue text, not injected HTML.

## Validation

- Focused normal storage/HTTP PASS0.303s/0.200s: actual interview/answer/offer/acceptance/leave approval, privacy, another candidate sees none, exclusive/end cursor, restart equality, foreign principal, revoked grant, negative cursor, no world-head change.
- Additional51actual-invitation boundary test exercises50+1 pagination without fake message rows. Targeted race storage9.984s/HTTP2.725s PASS; storage/HTTP vet PASS. Full HTTP regression PASS10.912s.
- Build/typecheck PASS.
- `node scripts/verify-rp6-work-life.mjs --messages`: PASS `/tmp/corerp-rp6-work-vYNVsZ`. Actual economic bootstrap, authorized creator organization and manager posting/invitation, Lin's actual application/answer; phone0→2 notices; process/browser restart returns exact same records. Browser verifies scoped event ownership/minimal DTO, safe stale snapshot/read retry, keyboard/Escape, no facts/knowledge/bookmark changes, no credential persistence or page errors. Actual mobile/desktop content screenshots inspected.
- Original real Play journey supports `--messages`; after ordinary speech/knowledge/travel it still returns no phone records, not relabeled hearing. All-current-increment combined browser64939 PASS `/tmp/corerp-rp1-e2e-bSz4h2`,33local HTTP fixture decision calls, with `--messages --map --work --contacts --wallet --style-ui --regenerate --turn-stream --context-budget --fake-model`.

Limitations: browser exercises invitation/answer; other notice families have the backend source boundary and selected integration coverage, not a claim that every family was separately browser-tested. No live LLM, publication or full RP6 acceptance implied. Custom narrative execution and100+turn/full-stage checks remain required.
