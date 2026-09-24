package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type StudioGenesisRequest struct {
	CreationPlanHash    string               `json:"creation_plan_hash,omitempty"`
	PrincipalID         string               `json:"principal_id"`
	AuthorityInstanceID string               `json:"authority_instance_id"`
	AuthorityBranchID   string               `json:"authority_branch_id"`
	InstanceID          string               `json:"instance_id"`
	IdempotencyKey      string               `json:"idempotency_key"`
	Spec                core.StudioWorldSpec `json:"spec"`
}
type StudioGenesisResult struct {
	InstanceID  string `json:"instance_id"`
	BranchID    string `json:"branch_id"`
	EventID     string `json:"event_id"`
	CohortID    string `json:"cohort_id"`
	RequestHash string `json:"request_hash"`
	Status      string `json:"status"`
	Replayed    bool   `json:"replayed"`
}

// PrepareStudioWorld creates source-backed paused genesis, not a playable world.
// Participants and validated package activation must finish before publication.
func (s *Store) PrepareStudioWorld(ctx context.Context, r StudioGenesisRequest) (StudioGenesisResult, error) {
	var empty StudioGenesisResult
	if err := validateStudioGenesisRequest(r); err != nil {
		return empty, err
	}
	requestHash, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	identity, err := core.HashJSON([]string{r.PrincipalID, r.AuthorityInstanceID, r.AuthorityBranchID, r.IdempotencyKey})
	if err != nil {
		return empty, err
	}
	command := "cmd_studio_genesis_" + identity[7:]
	id := func(kind, key string) string { v, _ := core.StudioWorldObjectID(r.InstanceID, kind, key); return v }
	branch, event, cohort := "br_main", id("event", "genesis"), id("cohort", "population")
	out := StudioGenesisResult{InstanceID: r.InstanceID, BranchID: branch, EventID: event, CohortID: cohort, RequestHash: requestHash, Status: "prepared"}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if err := authorizeStudioWorldCreation(ctx, tx.conn, r.PrincipalID, r.AuthorityInstanceID, r.AuthorityBranchID); err != nil {
		return empty, err
	}
	var oldHash, status string
	err = tx.conn.QueryRowContext(ctx, `SELECT request_hash,status FROM commands WHERE command_id=?`, command).Scan(&oldHash, &status)
	if err == nil {
		if oldHash != requestHash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "genesis key used for another specification or target")
		}
		if status != "committed" {
			return empty, core.NewError(core.CodeCommandInProgress, "genesis is not committed")
		}
		out.Replayed = true
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	var existing int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_instances WHERE instance_id=?`, r.InstanceID).Scan(&existing); err != nil {
		return empty, err
	}
	if existing != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "target world already exists")
	}
	lock := struct {
		Version string `json:"version"`
		Status  string `json:"status"`
	}{"corerp.studio-genesis.v1", "awaiting_package_activation"}
	lockJSON, err := core.CanonicalJSON(lock)
	if err != nil {
		return empty, err
	}
	ruleHash, err := core.HashJSON(lock)
	if err != nil {
		return empty, err
	}
	fact := struct {
		Request  StudioGenesisRequest `json:"request"`
		CohortID string               `json:"cohort_id"`
	}{r, cohort}
	payload, err := core.CanonicalJSON(fact)
	if err != nil {
		return empty, err
	}
	batchHash, err := core.HashJSON(struct {
		Command string `json:"command_id"`
		Payload any    `json:"payload"`
	}{command, fact})
	if err != nil {
		return empty, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	at := r.Spec.StartWorldTime
	batch, attempt, epoch := id("batch", "genesis"), id("attempt", "genesis"), "epoch_0"
	exec := func(q string, args ...any) error { return execAgentOne(ctx, tx.conn, "Studio genesis", q, args...) }
	statements := []struct {
		q string
		a []any
	}{
		{`INSERT INTO world_instances VALUES (?,'corerp.studio.world','1.0.0',?,'paused')`, []any{r.InstanceID, now}},
		{`INSERT INTO branches(instance_id,branch_id,label,head_sequence,created_at_utc) VALUES (?,?,?,0,?)`, []any{r.InstanceID, branch, r.Spec.Name, now}},
		{`INSERT INTO rule_epochs(instance_id,branch_id,epoch_id,start_sequence,ruleset_hash,lock_document) VALUES (?,?,?,1,?,?)`, []any{r.InstanceID, branch, epoch, ruleHash, string(lockJSON)}},
		{`INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'PrepareStudioWorld',?,?,0,?,'{"authorization":"sourced-world-create"}','pending',?)`, []any{command, r.InstanceID, branch, r.IdempotencyKey, requestHash, r.PrincipalID, now}},
		{`INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-studio',?,?,?)`, []any{command, attempt, s.now().UTC().Add(time.Minute).Format(time.RFC3339Nano), requestHash, now}},
		{`INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,0,1,1,1,?,?,?)`, []any{batch, command, r.InstanceID, branch, epoch, at, batchHash, now}},
		{`INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,1,0,'StudioWorldPrepared',?,?,?)`, []any{event, batch, r.InstanceID, branch, r.PrincipalID, at, string(payload)}},
		{`INSERT INTO currencies(currency_id,scale,symbol,definition_event_id) VALUES (?,0,'cr',?)`, []any{id("currency", "credit"), event}},
		{`INSERT INTO product_skus(sku_id,base_unit,quantity_scale,definition_event_id) VALUES (?,'unit',0,?)`, []any{id("sku", "staple"), event}},
	}
	for _, st := range statements {
		if err := exec(st.q, st.a...); err != nil {
			return empty, err
		}
	}
	currency, sku := id("currency", "credit"), id("sku", "staple")
	for _, a := range []struct {
		key, kind string
		balance   int64
	}{{"asset", "asset", r.Spec.OpeningMoneyMinor}, {"receivable", "receivable", 0}, {"liability", "liability", 0}, {"issuance", "issuance_source", -r.Spec.OpeningMoneyMinor}} {
		account := id("account", a.key)
		owner := cohort
		if a.kind == "issuance_source" {
			owner = id("authority", "monetary")
		}
		if err := exec(`INSERT INTO accounts(account_id,owner_id,currency_id,account_type,overdraft_limit_minor,opened_by_event_id) VALUES (?,?,?,?,0,?)`, account, owner, currency, a.kind, event); err != nil {
			return empty, err
		}
		if err := exec(`INSERT INTO account_balances VALUES (?,?,0,1)`, account, a.balance); err != nil {
			return empty, err
		}
	}
	location, source := id("location", "cohort"), id("location", "source")
	if err := exec(`INSERT INTO stock_locations(location_id,owner_id,location_kind) VALUES (?,?,'holder')`, location, cohort); err != nil {
		return empty, err
	}
	if err := exec(`INSERT INTO stock_locations(location_id,owner_id,location_kind,capability_id) VALUES (?,?,'source','cap_inventory_create')`, source, r.InstanceID); err != nil {
		return empty, err
	}
	for _, l := range []struct {
		id     string
		amount int64
	}{{location, r.Spec.OpeningStockMinor}, {source, 0}} {
		if err := exec(`INSERT INTO inventory_balances VALUES (?,?,?,0,1)`, l.id, sku, l.amount); err != nil {
			return empty, err
		}
	}
	if err := exec(`INSERT INTO cohorts(cohort_id,instance_id,branch_id,display_name,population_count,asset_account_id,receivable_account_id,liability_account_id,inventory_location_id,currency_id,sku_id,allocation_algorithm_version,status,projection_version,last_event_sequence,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,'equal-share-v1','active',0,1,?)`, cohort, r.InstanceID, branch, r.Spec.Name, r.Spec.Population, id("account", "asset"), id("account", "receivable"), id("account", "liability"), location, currency, sku, event); err != nil {
		return empty, err
	}
	if r.Spec.OpeningMoneyMinor > 0 {
		entry := id("journal", "genesis")
		if err := exec(`INSERT INTO journal_entries VALUES (?,?,'draft','Studio opening issuance')`, entry, event); err != nil {
			return empty, err
		}
		for _, p := range []struct {
			key    string
			amount int64
		}{{"asset", r.Spec.OpeningMoneyMinor}, {"issuance", -r.Spec.OpeningMoneyMinor}} {
			if err := exec(`INSERT INTO postings(posting_id,entry_id,account_id,currency_id,amount_minor,memo) VALUES (?,?,?,?,?,'Studio opening issuance')`, id("posting", p.key), entry, id("account", p.key), currency, p.amount); err != nil {
				return empty, err
			}
		}
		if err := exec(`UPDATE journal_entries SET status='posted' WHERE entry_id=?`, entry); err != nil {
			return empty, err
		}
	}
	if r.Spec.OpeningStockMinor > 0 {
		if err := exec(`INSERT INTO stock_movements(movement_id,event_id,sku_id,from_location_id,to_location_id,quantity_minor,movement_kind,reason_code,capability_id) VALUES (?,?,?,?,?,?,'create','studio_genesis','cap_inventory_create')`, id("stock", "genesis"), event, sku, source, location, r.Spec.OpeningStockMinor); err != nil {
			return empty, err
		}
	}
	if err := exec(`INSERT INTO population_movements(movement_id,event_id,to_owner_kind,to_owner_id,population_count,movement_kind,reason_code) VALUES (?,?,'cohort',?,?,'create','studio_genesis')`, id("population", "genesis"), event, cohort, r.Spec.Population); err != nil {
		return empty, err
	}
	if err := exec(`INSERT INTO world_clocks VALUES (?,?,?,0,'paused',0,1)`, r.InstanceID, branch, at); err != nil {
		return empty, err
	}
	if err := s.insertScopedAudit(ctx, tx.conn, r.InstanceID, branch, id("audit", "genesis"), "runtime_diagnostic", event, command, attempt, at, now, payload); err != nil {
		return empty, err
	}
	for _, st := range []struct {
		q string
		a []any
	}{
		{`UPDATE branches SET head_sequence=1 WHERE instance_id=? AND branch_id=? AND head_sequence=0`, []any{r.InstanceID, branch}},
		{`UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE command_id=? AND attempt_no=1`, []any{now, command}},
		{`UPDATE commands SET status='committed' WHERE command_id=?`, []any{command}},
	} {
		if err := exec(st.q, st.a...); err != nil {
			return empty, err
		}
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return out, nil
}
