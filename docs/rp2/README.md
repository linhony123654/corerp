# RP-2 — 人物生命层

Current: RP-2A provider integration; RP-2B–E not yet complete. The overall living-world goal continues through RP-8 and final 300-turn/30-day integration, strictly serially.

## Real DecisionProvider configuration

The default remains deterministic and makes no model network request. The server can now use an operator-configured Chat Completions compatible endpoint supporting strict JSON-schema output. Configure the complete request URL and a model available to that endpoint; no model name or remote service is silently selected.

```bash
export CORERP_DECISION_PROVIDER=chat_completions
export CORERP_LLM_ENDPOINT=https://api.openai.com/v1/chat/completions
export CORERP_LLM_MODEL='<your compatible model>'
export CORERP_LLM_API_KEY='<your provider key>'
export CORERP_LLM_TIMEOUT=10s
export CORERP_LLM_ATTEMPTS=2
```

Start the backend as in [Play setup](../rp1/play.md), with its separate player-auth configuration. Never put provider credentials in frontend settings or commit them. The adapter uses only these explicit CoreRP variables; it never reads another application's credentials. Local loopback HTTP/keyless model servers are supported; remote endpoints require HTTPS and a key. Partial, unsupported or contradictory settings fail startup. Redirects are refused, including same-host redirects, so use the final request URL. Query-string/userinfo credentials are rejected.

The adapter sends `messages` and `response_format.type=json_schema` with `strict=true`, a closed object containing exactly `action`, `text`, `destination_place_id`, `stream=false`, `store=false`, and a 1024 completion-token bound. This follows the [official structured output contract](https://developers.openai.com/api/docs/guides/structured-outputs) and [Chat Completions API](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create). Compatible providers must actually support these fields; unsupported dialects fail safely rather than silently weakening output validation. No OpenAI SDK dependency is required; the Go adapter implements the HTTP contract.

## Data / authority boundary

- Structured `corerp.decision.v1` context wraps the existing NPC-scoped input. It contains own state/asset balance, own observed Knowledge, next schedule if any, visible presence and legal actions/routes. Nonexistent relationship/memory systems are not fabricated by RP-2A; those are subsequent stages.
- Character/player text is JSON data in a separate user message, never interpolated into system instructions. A model cannot query storage or execute tools. No creator prompt, other NPC assets, unseen event stream or DB is provided.
- Local parsing rejects unknown/missing/duplicate/null/non-string fields, trailing JSON, truncated/refused/tool-call responses and oversized bodies. A shared domain validator rejects illegal actions, unreachable destinations, mixed effects and invalid speech, even if the remote server ignores the schema.
- The original authoritative commit independently revalidates world state before writing. Accepted player speech remains true when a provider fails; the NPC effect safely becomes an audited silence through normal validation/commit. No incomplete model output is saved as speech or used as a currency/location fact.
- Total timeout includes retries/backoff (default 10s, maximum 60s); at most 3 attempts. Only network interruptions, 408/429/5xx are retried. Invalid output, authentication errors and redirects are not retried. Retry-After cannot outlive the total budget. Raw provider bodies, URLs and credentials are excluded from diagnostic errors/audit.
- Already committed NPC effects are not re-requested after restart. An uncommitted model request may be attempted again; provider billing/external computation cannot be made atomic with the local SQLite transaction. Missing effects resume under the server's currently configured provider; historical committed facts remain stable.
- Play indicates the configured mode after authenticated observation. It does not expose the endpoint, model key or private NPC context.

## Verification / live status

`npm run verify:rp2-provider` starts actual Go HTTP/SQLite and browser processes plus a **local fake model HTTP server**. It proves server configuration → filtered request → strict response → world commit → Play → process/browser recovery with no repeated model calls for committed effects. It does not prove live model quality. The original `npm run verify:rp1-play` explicitly selects deterministic mode, and both verifiers erase inherited provider credentials from child environments.

Live model E2E: **REQUIRED_IF_AVAILABLE / NOT VERIFIED**. No CoreRP endpoint/model/key is configured in the current task environment. Implementation and fake-provider verification continue as explicitly permitted by the goal. [RP-2A report](phase-2a.md).
