# RP6 own work and agenda

`POST /api/v1/rp/work/read` accepts an authenticated `session_id`. Every read verifies the caller's active session, live control grant and entity/world binding. One transaction reads the current world clock/head, own employment and pending agenda. No events, decisions, messages, journal entries or action bookmarks are created.

Jobs reuse existing wage participation and individual employment contracts plus accepted Career terms. The public contract contains contract ID, employer/workplace names when registered, status, exact decimal-string `wage_minor`, declared pay period and currency metadata. Internal evaluations, capabilities, account IDs and other people's contracts are excluded. Legacy participation contracts do not declare a pay period or necessarily have a display name; the UI explicitly reports these as unregistered. Wage is the contractual amount, not wallet balance or proof of payment. A future-effective wage participation is not shown before it begins.

The shared employment reader now takes explicit instance/branch for aggregate-exit filtering instead of demo constants. All existing callers provide their actual scope, including background creation before the profile exists. This does not introduce a new employment authority or migration.

Agenda lists up to20 active own schedule entries with pending scheduler items at/after the current world time, sorted by effective time, priority and ID. A21st lookahead sets `more_appointments`; this is an upcoming window, not a complete paginated archive. Effective execution time comes from the scheduler. If different from the original schedule, matching actor/original/retry transit evidence is required; otherwise the read fails with projection divergence. Both original and effective times are displayed when delayed. Agenda may include home/presence activities, not just work shifts.

`PlayWork.vue` opens on demand from Play as one paper sheet, centered on desktop/bottom-aligned on phone. Contract and agenda are separate ruled sections. Loading, empty, read failure/retry, labeled stale snapshot,44px controls, keyboard containment/Escape and ignored late results follow the existing personal views. Reads do not use the durable world-action retry mechanism.

## Verification

- Storage tests: actual Career offer acceptance, own wage/status/period/workplace, exclusion of private recruitment data, start-time transition, process reopen equality, foreign principal and revoked grant; no branch-head mutation. Added actual30-day agenda bound and20-item notice contract, actual roadworks delay, preserved original time and fail-closed damaged queue test. Focused normal work tests PASS1.816s.
- HTTP tests: actual own session, nonnil empty arrays, authentication, foreign-session privacy and spoofed principal rejection. Full HTTP regression PASS11.599s; storage/HTTP vet PASS.
- Affected shared helper regression with race: work/background/culture/CareerAcceptance/CareerAggregate PASS storage258.650s/HTTP4.321s. This run preceded the additional delay test; its focused race evidence is recorded in progress.
- `node scripts/verify-rp6-work-life.mjs`: PASS, artifacts `/tmp/corerp-rp6-work-tfCyFF`. Real temporary economic bootstrap; before-effective empty → six real4-hour UI waits → own wage participation visible; exact SQLite wage match; actual process/browser restart equality; errors/retry, focus/Escape, snapshot nonmutation and no credential persistence. No live LLM. Default Lin has no declared agenda; populated/delayed agenda is verified by storage tests, not falsely claimed as a populated browser screenshot.
- Additional delay/bound/projection-damage focused race PASS3.727s and storage vet PASS.
- Original full Play journey accepts `--work` for empty work views before and after its real dialogue/travel/wait/recovery flow. Combined `--work --contacts --wallet --style-ui --regenerate --turn-stream --context-budget --fake-model` PASS, `/tmp/corerp-rp1-e2e-762DT5`,27local HTTP fixture decision calls, not a live model.

Actual funded phone390×844 and desktop1440×1000 screenshots inspected: readable title/status/amount, explicit missing-period and payment caveats, separate empty-agenda state, clear focus/close/refresh; no overflow or dashboard card grid. Existing visual identity retained without extra assets/dependencies.

This increment does not satisfy messages/map, custom prose execution,100+turn or the full RP6 gate. No stage commit or deployment yet.
