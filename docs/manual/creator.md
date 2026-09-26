# CoreRP Creator Manual

Welcome to the Creator's Guide for CoreRP. As a creator, you author the institutions, spatial geographies, demographic populations, economic policies, and DLC content packs that shape living worlds.

---

## 1. World Creation & Genesis

Creating a new world establishes an isolated branch with root currencies, law systems, and an initial world clock.

### Creating a World via Studio

```bash
curl -X POST http://localhost:8080/api/v1/studio/worlds \
  -H "Authorization: Bearer principal_creator" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_new_colony",
    "branch_id": "br_main",
    "world_label": "Verdant Outpost Colony",
    "base_currency_id": "credit",
    "initial_world_time": "2026-10-01T08:00:00Z"
  }'
```

The genesis command initializes the double-entry accounting ledger accounts (`issuance`, `treasury`), the initial rule epoch (`epoch_0`), and registers the root branch head.

---

## 2. Spatial Topology & Location Hierarchy

CoreRP organizes space as a concrete, navigable tree overlaid with directed transit edges:

### Defining Places and Location Nodes

- **Root Places (`agent_places`)**: Coarse spatial anchors (e.g., `place_market_district`, `place_residential_block`).
- **Location Nodes (`rp_location_nodes`)**: Structured hierarchical slots within a place (e.g., `/place_residence/room_101/desk`).

```sql
INSERT INTO agent_places(place_id, instance_id, branch_id, display_name, place_kind, status, definition_event_id)
VALUES ('place_bakery', 'inst_new_colony', 'br_main', 'Old Town Bakery', 'work', 'active', 'event_genesis');

INSERT INTO rp_location_nodes(location_id, instance_id, branch_id, parent_location_id, slot_key, readable_path, generator_version, definition_event_id)
VALUES ('loc_bakery_counter', 'inst_new_colony', 'br_main', 'place_bakery', 'counter', '/bakery/counter', 'v1', 'event_genesis');
```

### Directed Timed Edges (`rp_timed_edges`)

To connect two distinct places with a travel duration:

```sql
INSERT INTO rp_timed_edges(edge_id, instance_id, branch_id, from_place_id, to_place_id, segment_place_id, duration_minutes, definition_event_id)
VALUES ('edge_home_to_work', 'inst_new_colony', 'br_main', 'place_residence', 'place_bakery', 'place_transit_way', 15, 'event_genesis');
```

---

## 3. Demographics, Households & Organizations

### Cohort Materialization

Rather than simulating millions of individual agents at all times, CoreRP manages demographic **Cohorts**:
- A Cohort represents an aggregate population pool with shared assets and liabilities.
- When an individual is needed for fine-grained interaction or employment, they are **materialized** as a concrete `materialized_entity`.

### Households

Households bundle adults and dependents under a unified residential roof:
- Every household owns a dedicated rent account (`rent_account_id`).
- Adults agree to contribution shares to fund recurring lease obligations.
- If income shocks occur (e.g., job loss), the household budget pressure forecast adjusts automatically (`covered`, `strained`, `critical`).

### Organizations & Career Structures

Organizations manage workplaces, post vacancies, and disburse payroll:
- **Positions**: Define job titles, grades (e.g., `junior` -> `senior`), wage rates, and shift hours (e.g., 08:00–17:00).
- **Agency Policies**: Creators or managers can configure automated review policies (`organization_agency_policies`) that dynamically freeze recruitment during downturns or expand capacity when cash reserves allow.

---

## 4. Pack Authoring, Installation & Activation

CoreRP supports pure-data content and narrative packs (DLCs) without requiring code modifications to the host engine.

### Pack Kinds

1. **`system`**: Core mechanics and ruleset definitions.
2. **`narrative`**: Presentation styles, tone, density, and perspective rules.
3. **`content`**: Real domain data packs (e.g., career catalogs, training programs, credential requirements).

### Pack Structure: Pure Declarative Data

A pack consists of two files:
- `manifest.json`: Declares package identity, kind, version, and content hash.
- `content.json` or `narrative.json`: The declarative data schema.

Example `manifest.json`:
```json
{
  "package_id": "pack_retail_career",
  "kind": "content",
  "version": "1.0.0",
  "engine_api": "corerp-v1",
  "schema_hash": "sha256:7a8b9c...",
  "content_hash": "sha256:3d4e5f..."
}
```

### Installation vs. Activation

- **Installation (`InstallStudioPackageLocal`)**: Validates the manifest and hashes, storing the immutable bundle on disk. It does not alter active world mechanics.
- **Activation (`ActivateStudioPackages`)**: Pins the installed packages into a new immutable **Rule Epoch** (`rule_epochs`), transitioning branch rules forward with deterministic provenance.

```bash
# Activate installed packages
curl -X POST http://localhost:8080/api/v1/studio/packages/activate \
  -H "Authorization: Bearer principal_creator" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_new_colony",
    "branch_id": "br_main",
    "system_package_id": "pack_core_system",
    "narrative_package_id": "pack_life_journal",
    "content_package_ids": ["pack_retail_career"]
  }'
```

---

## 5. Studio Inspector

The Studio Inspector is the creator's time-travel magnifying glass for diagnosing any historical world state.

### Inspecting an Event

Every event in the world can be queried with its full causation chain and epoch rule provenance:

```bash
curl -X POST http://localhost:8080/api/v1/studio/inspect/event \
  -H "Authorization: Bearer principal_creator" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_id": "principal_creator",
    "instance_id": "inst_new_colony",
    "branch_id": "br_main",
    "event_id": "event_987654"
  }'
```

The response includes:
- `event_sequence` & `world_time`
- `batch_hash` & `ruleset_hash`
- Pinned package locks active when the event was committed
- Causation event link (the preceding event that triggered this one)
- Diagnostic evidence (why NPCs made specific choices)

---

## 6. Safe Branching & What-If Experiments

Because CoreRP uses append-only event sourcing, creators can branch from any past sequence without modifying production history:

```bash
curl -X POST http://localhost:8080/api/v1/studio/branches/fork \
  -H "Authorization: Bearer principal_creator" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_new_colony",
    "source_branch_id": "br_main",
    "new_branch_id": "br_experiment_reform",
    "fork_at_sequence": 1450
  }'
```

On the new branch, you can test alternative tax laws, economic shocks, or pack upgrades completely safely.
