# CoreRP Player Manual

Welcome to the player's guide for CoreRP. This guide walks you through setting up, joining a living simulation world, interacting with other characters, navigating physical spaces, managing your daily life (work, rent, expenses), and understanding how character knowledge differs from objective world reality.

---

## 1. Installation & Starting CoreRP

CoreRP can be run locally using the compiled Go backend server and the web client.

### Starting the Server

```bash
# Compile and run the backend server
/usr/local/go/bin/go run ./backend/cmd/corerp-server --db /tmp/my-world.db --port 8080
```

When started with a new database path, the server automatically applies all database migrations and seeds initial administrative principals.

### Launching the Play Web Interface

From the repository root:

```bash
cd corerp-console
npm install
npm run dev
```

Navigate to `http://localhost:5173` in your browser.

---

## 2. Creating and Selecting Worlds

CoreRP organizes reality into **World Instances** and **Branches**:

- **Instance (`instance_id`)**: A distinct universe with its own demographic cohorts, laws, and economic currencies.
- **Branch (`branch_id`)**: A timeline within that instance. The canonical main branch is typically `br_main`.

### Selecting an Existing World

In the Play UI or via API, choose your active branch. For the standard living world demo, use:
- Instance: `inst_m2_t09`
- Branch: `br_main`

---

## 3. The Core Play Loop: Roleplay Sessions

Roleplay sessions represent your temporary interface to pilot a character. Opening a session connects your client credentials to a physical character entity.

### Opening a Session

```bash
curl -X POST http://localhost:8080/api/v1/rp/session/open \
  -H "Authorization: Bearer principal_m2_rp_player" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_m2_t09",
    "branch_id": "br_main",
    "entity_id": "entity_m2_rp_lin",
    "pov": "second_person",
    "idempotency_key": "my-first-session"
  }'
```

### Point of View (POV) Settings

You can configure how narrative scenes are described to you:
- `"first_person"`: Described from the character's direct perspective ("I walk into the cafe and order tea.")
- `"second_person"`: Classic interactive narrative ("You walk into the cafe and order tea.")
- `"third_person"`: Objective literary narrative ("Lin walks into the cafe and orders tea.")

---

## 4. Interaction: Dialogue, Scenes & Long Narrative

### Spoken Dialogue (`UtterRP`)

When your character speaks in a room, all co-located characters in the same place hear the speech and receive an authoritative observation record.

```bash
curl -X POST http://localhost:8080/api/v1/rp/turn/utter \
  -H "Authorization: Bearer principal_m2_rp_player" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_id": "principal_m2_rp_player",
    "session_id": "sess_123456",
    "speech_text": "Good morning Ada. Did the shipment of tea arrive today?",
    "action_text": "Lin checks the display shelves while waiting.",
    "expected_cursor": 1,
    "idempotency_key": "turn-morning-01"
  }'
```

### Switching to Long-Form Narrative

CoreRP supports switching presentation styles between concise tactical output and rich, descriptive literary prose.

When an installed **Narrative Pack** (such as `life-journal`) is activated:
- Set `narrative_density: "long"` in your session style preferences.
- Responses will render extensive descriptions of atmosphere, sensory details, and character reactions without altering underlying world events.

---

## 5. Spatial Navigation: Move & Wait

### Moving Between Places (`MoveRP`)

Characters do not teleport; they navigate along established routes (`rp_place_links`) or timed transit edges (`rp_timed_edges`):

```bash
curl -X POST http://localhost:8080/api/v1/rp/move \
  -H "Authorization: Bearer principal_m2_rp_player" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_id": "principal_m2_rp_player",
    "session_id": "sess_123456",
    "from_place_id": "place_apartment_101",
    "to_place_id": "place_cafe",
    "expected_cursor": 2,
    "idempotency_key": "move-to-cafe"
  }'
```

### Advancing World Time (`WaitRP`)

To wait for an appointment, advance time until a work shift, or let other characters take initiatives:

```bash
curl -X POST http://localhost:8080/api/v1/rp/wait \
  -H "Authorization: Bearer principal_m2_rp_player" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_id": "principal_m2_rp_player",
    "session_id": "sess_123456",
    "target_world_time": "2026-09-22T14:00:00Z",
    "budget": 100,
    "expected_cursor": 3,
    "idempotency_key": "wait-afternoon"
  }'
```

---

## 6. Daily Life: Wallet, Work, Contacts & Messages

### Wallet & Account Balances

Characters have private accounts stored in the double-entry accounting ledger. Your wallet balance reflects your disposable cash:
- You receive wages upon completing work shifts and payroll cycles.
- Rent contributions are deducted from your household's shared rent account.
- Retail purchases at storefronts transfer currency directly from your account to the store merchant.

### Work & Career

Jobs are real economic contracts in CoreRP:
- **Job Board**: View open positions posted by organizations.
- **Applying & Interviews**: Submit applications and attend interviews conducted by managers.
- **Shifts & Duties**: Arrive at your workplace during scheduled shift hours (e.g., 08:00–17:00) to accrue attendance and earn wages.

### Contacts & Relationships

CoreRP tracks interpersonal familiarity:
- Characters do not automatically know the names of strangers simply by being in the same room.
- Introduced characters become known contacts recorded in your `rp_identity_familiarity` ledger.
- Use interpersonal actions (`greet`, `gift`, `promise_meeting`) to develop bonds.

### Messages & Channels

Communication in CoreRP respects real channels:
1. **Face-to-face speech**: Heard only by characters in the same physical room.
2. **Direct messages**: Private messages sent directly to specific known contacts.
3. **Organization notices**: Announcements issued by employer management to employees.
4. **Public notices**: Official declarations published pursuant to enacted laws.

---

## 7. Retry, Replay & Narrative Regeneration

### Safe Retries (Idempotency)

Every modifying command accepts an `idempotency_key`. If your network drops or a request times out, re-submitting the exact same request with the same key is completely safe:
- CoreRP recognizes duplicate requests and returns the original committed receipt.
- No duplicate events, double spending, or redundant moves will ever be recorded.

### Regenerating Narrative Presentation

If you want to view a past scene described in a different tone, literary POV, or density:
- You can request presentation regeneration.
- The underlying world facts, balances, and event IDs remain **strictly unchanged**. Only the cosmetic rendering is re-evaluated.

---

## 8. World Truth vs. Character Claims

One of the foundational tenets of CoreRP is the strict separation between:

| Concept | Meaning in CoreRP |
| --- | --- |
| **World Truth** | The objective, immutable events recorded in the SQLite ledger (actual transfers, physical movements, committed contracts). |
| **Sourced Observation** | What an individual character's senses actually recorded (who was seen in the room, what words were spoken aloud). |
| **Character Belief** | What a character believes based on rumors, claims, or deductions. |
| **Utterance / Claim** | What a character says out loud. Characters can lie, exaggerate, or be misinformed. |

An NPC claiming *"I already paid you the rent"* does not deduct money from their account or satisfy an obligation unless an actual financial transfer event exists in the ledger!
