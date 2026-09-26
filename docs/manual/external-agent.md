# CoreRP External Agent & Model Context Protocol (MCP) Guide

Welcome to the External Agent guide for CoreRP. This manual explains how autonomous Large Language Models (LLMs) and external AI agents connect to CoreRP via Anthropic's **Model Context Protocol (MCP)**, pilot characters, and interact with the world under strict epistemic fencing.

---

## 1. Overview & Architecture

CoreRP allows autonomous external models to pilot living residents alongside Human players and Core heuristic NPCs. To ensure fairness, verifiability, and world consistency, all external agents interact through the **Model Context Protocol (MCP)**:

```text
External LLM (Claude, etc.)
        │
        ▼ (stdio / JSON-RPC)
  MCP Adapter (clients/mcp/server.js)
        │
        ▼ (Authenticated HTTP / SSE)
  CoreRP Engine (backend)
        │
        ▼ (Deterministic Validation)
  Authoritative SQLite Event Ledger
```

---

## 2. MCP Server Configuration

The CoreRP MCP server lives in `clients/mcp/` and communicates over standard input/output (`stdio`).

### Adding CoreRP to your MCP Client Config

In your `claude_desktop_config.json` or MCP client configuration:

```json
{
  "mcpServers": {
    "corerp": {
      "command": "node",
      "args": [
        "/path/to/corerp/clients/mcp/server.js"
      ],
      "env": {
        "CORERP_ENDPOINT": "http://localhost:8080",
        "CORERP_TOKEN": "principal_mcp_resident_ada"
      }
    }
  }
}
```

### Verifying Connection

Once configured, the MCP client will discover the suite of CoreRP tools:
- `corerp_observe`: Fetch the current sensory observation for your character.
- `corerp_speak`: Utter dialogue in the current room.
- `corerp_move`: Navigate to a connected location.
- `corerp_wait`: Yield your turn and advance world time.
- `corerp_interact`: Perform interpersonal actions (greet, gift, apologize).
- `corerp_apply_job`: Apply for an open career posting.

---

## 3. Controller Enrollment & Exclusive Ownership

To prevent two controllers from issuing conflicting actions for the same character simultaneously, CoreRP enforces **Controller Ownership**:

```bash
curl -X POST http://localhost:8080/api/v1/rp/controller/enroll \
  -H "Authorization: Bearer principal_mcp_operator" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_m2_t09",
    "branch_id": "br_main",
    "entity_id": "entity_m2_rp_ada",
    "controller_kind": "mcp_external_model",
    "controller_instance_id": "mcp_runner_claude_opus",
    "lease_seconds": 300
  }'
```

- **Exclusive Control**: While enrolled, only requests bearing the registered controller lease can move or speak for the character.
- **Heartbeats**: The controller must renew its lease periodically. If the controller crashes or disconnects, the lease expires safely.
- **Heuristic Fallback**: When an external controller disconnects, the character automatically resumes their default autonomous NPC schedule without getting stuck.

---

## 4. Limited Observation & Epistemic Fencing

CoreRP strictly prohibits models from acquiring **omniscient knowledge**.

When calling `corerp_observe`, the model receives only what is physically observable:

```json
{
  "current_place": "Old Town Bakery",
  "world_time": "2026-10-01T08:15:00Z",
  "present_actors": [
    {
      "entity_id": "entity_m2_rp_lin",
      "display_name": "Lin",
      "activity": "ordering_coffee"
    }
  ],
  "recent_utterances": [
    {
      "speaker": "Lin",
      "text": "Good morning Ada! Do you have any fresh bread?"
    }
  ],
  "personal_state": {
    "fatigue_level": "well_rested",
    "wallet_balance_minor": 450
  }
}
```

### Strict Fencing Guardrails

- **No Third-Party Financial Balances**: Models never see other characters' bank accounts or debt obligations.
- **No Secret NPC Reasoning**: An external model cannot read another character's internal prompts, motives, or hidden health conditions.
- **Unverified Information Remains Claims**: Messages received from other characters or rumors are labeled `unverified`, preserving the distinction between world truth and character hearsay.

---

## 5. Synchronized Time & Shared Action Rounds

In multiplayer scenes where a Human player and external AI residents are present in the same room, CoreRP uses **Shared Rounds** (`rp_shared_rounds`):

1. **Round Open**: The server opens an action window for the room.
2. **Proposals Collected**: Each participant (Human, MCP model, heuristic NPC) submits their intended action or wait intent.
3. **Fair Selection & Commitment**: The authoritative scheduler commits an action, resolves co-location effects, and advances room time deterministically.
4. **Resolution**: All participants receive updated observations simultaneously.

This prevents fast network clients from spamming turns or starving slower AI model loops.

---

## 6. Action Budgets & Graceful Disconnection

To prevent runaway API consumption or infinite loops:
- Every agent profile carries an `action_budget_per_day` (e.g., 50 actions per world day).
- When the daily budget is exhausted, the character enters a restful wait state until the next world morning.
- To safely release a character:

```bash
curl -X POST http://localhost:8080/api/v1/rp/controller/release \
  -H "Authorization: Bearer principal_mcp_operator" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_m2_t09",
    "branch_id": "br_main",
    "entity_id": "entity_m2_rp_ada"
  }'
```
