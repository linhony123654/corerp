# RP request retirement

`POST /api/v1/rp/requests/retire` uses the existing authenticated JSON envelope.
Input: `operation` (`open`, `dialogue`, `wait`, `move`, `social`), original
`idempotency_key`, original `session_id` (absent for open). Authentication supplies
the principal; arbitrary principal/session overrides are forbidden.

This mutates request acceptance; it is not a read-only query or cancellation.
Response includes `protocol_version`, operation, key, session when known, status:

- `retired`: no owner accepted the request, and that key is permanently disabled.
  Only this result allows discarding an ambiguous pending request without replay.
- `in_progress`: the original owner accepted work; preserve the exact original
  request and recover it. Effects may already exist. No rollback/cancel occurred.
- `completed`: replay the original request to recover its result; do not issue
  the action under a new key. Open returns the existing session ID, even if closed.

Error, timeout, absence or ordinary HTTP rejection is not proof of non-acceptance.
Retrying retirement with the same scope/key is safe. Retirement does not replace
payload-hash mismatch checks; it disables all bodies using that operation/key.

Schema027 adds an immutable application fence, not a world fact or projection.
Retirement and all five original acceptance owners use immediate transactions;
whichever commits first determines the outcome. Dialogue/wait intents are checked
before final receipts, covering partial speech/scheduler work. Move/social inspect
commands without requiring an event join. No Event/time/cursor/character/economic
state is written. Projection rebuild retains fences; reopening enforces them.

Session actions require own session and current RP-control authority. Closed
sessions may resolve prior requests; recovery grants no new action authority.
Open retirement needs the existing caller principal only: the original open may
have failed for an invalid world/entity. It cannot disable another caller's key.

The installed adapter exposes “停用未接受请求”. It validates the returned scope,
retains the exact original pending request for accepted outcomes or ambiguous
failures, and verifies host acknowledgement persistence before considering local
recovery complete. After reload an invalid open can be retired directly by
entering a token, without implicitly retrying that open.

## Verification

- Storage all-five-owner both-order checks, separate-connection concurrency,
  rebuild/reopen, immutable fences, scope/permissions, partial speech/wait recovery
  and026upgrade; HTTP auth/envelope/late request409. Relevant race78947 PASS
  storage42.355s/HTTP2.773s, including older migration fixtures.
- Existing affected storage session/move/social/wait/turn regression60630 PASS
  5.079s. FullHTTP/server51397 PASS14.370s/0.206s; targeted vet PASS.
- Final026upgrade test also compares actual before/after replay StateHash;
  focused race34467 PASS2.829s. Only this test assertion changed afterward;
  previous runtime, HTTP and actual-host evidence remains current.
- Real installed SillyTavern1.19.0/runtime/browser26966 PASS,
  `/tmp/corerp-rp7-extension-drMdju`: invalid wait remains pending; real fence with
  lost reply preserves pending; delayed original is denied409; failed host save
  preserves pending; repeated retirement clears safely. Head/time/events/individuals
  unchanged. Accepted lost-response speech refuses retirement and exact retry
  creates no duplicate. Previous restart/actions/stream/chat-switch/budget-wait
  assertions remain passing. Desktop panel inspected; controls wrap within host.
- Expanded final real-host32340 PASS `/tmp/corerp-rp7-extension-cJ6RJP` also
  persists a rejected invalid-world open, reloads the actual host, reselects the
  same chat, enters a new memory-only token and retires directly without any open
  request being resent. Then a valid new binding succeeds. All prior assertions
  retained; fixture servers/browser closed, no live test handles remain.

No MCP/three-client7D or full RP7 completion claim. No production schema migration,
deployment or world rollback was performed; all migration tests used disposable DBs.
