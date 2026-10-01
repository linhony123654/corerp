package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
	_ "modernc.org/sqlite"
)

const (
	BaseSchemaVersion                        = "corerp-m0-draft-2026-09-22"
	RecoverySchemaVersion                    = "corerp-m1-recovery-002-2026-09-22"
	StrictSchemaVersion                      = "corerp-m1-strict-world-003-2026-09-22"
	AccountingSchemaVersion                  = "corerp-m1-obligation-accounting-004-2026-09-22"
	AuthorizationSchemaVersion               = "corerp-m1-authorization-issuance-005-2026-09-22"
	CohortSchemaVersion                      = "corerp-m2-cohort-materialization-006-2026-09-22"
	AgentSchemaVersion                       = "corerp-m2-agent-life-007-2026-09-22"
	EconomySchemaVersion                     = "corerp-m2-background-economy-008-2026-09-23"
	StoreSchemaVersion                       = "corerp-m2-finite-store-009-2026-09-23"
	SupplySchemaVersion                      = "corerp-m2-consumption-supply-010-2026-09-23"
	ArrearsSchemaVersion                     = "corerp-m2-arrears-grace-011-2026-09-23"
	InsolvencySchemaVersion                  = "corerp-m2-insolvency-012-2026-09-23"
	ClaimsSchemaVersion                      = "corerp-m2-bankruptcy-claims-013-2026-09-23"
	AllocationSchemaVersion                  = "corerp-m2-claim-allocations-014-2026-09-23"
	EstateSchemaVersion                      = "corerp-m2-estate-distribution-015-2026-09-23"
	WageParticipationSchemaVersion           = "corerp-m2-wage-participation-016-2026-09-23"
	WageAllocationSchemaVersion              = "corerp-m2-wage-allocation-017-2026-09-23"
	WageClaimOwnershipSchemaVersion          = "corerp-m2-wage-claim-ownership-018-2026-09-23"
	BankruptcySlotSchemaVersion              = "corerp-m2-bankruptcy-slot-claims-019-2026-09-23"
	RPSessionSchemaVersion                   = "corerp-rp1-sessions-020-2026-09-23"
	RPRouteSchemaVersion                     = "corerp-rp1-routes-021-2026-09-23"
	RPWaitSchemaVersion                      = "corerp-rp1-wait-intents-022-2026-09-23"
	RPSpeechSchemaVersion                    = "corerp-rp1-utterances-023-2026-09-23"
	RPNPCDecisionSchemaVersion               = "corerp-rp1-npc-decisions-024-2026-09-23"
	RPTurnSchemaVersion                      = "corerp-rp1-turn-runs-025-2026-09-23"
	RPStyleSchemaVersion                     = "corerp-rp2-styles-026-2026-09-23"
	RPRequestSchemaVersion                   = "corerp-rp7-request-retirement-027-2026-09-24"
	StudioPackageSchemaVersion               = "corerp-rp8-package-content-028-2026-09-24"
	StudioActivationSchemaVersion            = "corerp-rp8-package-activation-029-2026-09-24"
	StudioReadySchemaVersion                 = "corerp-rp8-world-ready-030-2026-09-24"
	RPInteractionSchemaVersion               = "corerp-f1-interactions-031-2026-09-25"
	RPSpatialLocationSchemaVersion           = "corerp-f2-spatial-locations-032-2026-09-25"
	RPSpatialJourneySchemaVersion            = "corerp-f2-spatial-journeys-033-2026-09-25"
	RPSpatialPerceptionSchemaVersion         = "corerp-f2-spatial-perception-034-2026-09-25"
	RPSpatialIdentitySchemaVersion           = "corerp-f2-spatial-identity-035-2026-09-25"
	RPControllerEnrollmentSchemaVersion      = "corerp-f3-controller-enrollment-036-2026-09-25"
	RPControllerAuthoritySchemaVersion       = "corerp-f3-controller-authority-037-2026-09-25"
	RPSharedRoundsSchemaVersion              = "corerp-f3-shared-rounds-038-2026-09-25"
	RPControllerLifecycleSchemaVersion       = "corerp-f3-controller-lifecycle-039-2026-09-25"
	RPSharedActionSchemaVersion              = "corerp-f3-shared-action-rounds-040-2026-09-25"
	RPSharedMoveSchemaVersion                = "corerp-f3-shared-move-rounds-041-2026-09-25"
	RPHouseholdSchemaVersion                 = "corerp-f4-households-042-2026-09-25"
	RPHouseholdRentSchemaVersion             = "corerp-f4-household-rent-043-2026-09-25"
	RPHouseholdContributionSchemaVersion     = "corerp-f4-household-rent-contributions-044-2026-09-25"
	RPHouseholdDependentSchemaVersion        = "corerp-f4-household-dependents-045-2026-09-25"
	RPSharedHealthSchemaVersion              = "corerp-f5-shared-health-rounds-046-2026-09-25"
	RPInformationSchemaVersion               = "corerp-f7-information-channels-047-2026-09-26"
	RPSharedInformationSchemaVersion         = "corerp-f7-shared-information-rounds-048-2026-09-26"
	RPSharedStanceSchemaVersion              = "corerp-f7-shared-information-stance-049-2026-09-26"
	RPSharedRelaySchemaVersion               = "corerp-f7-shared-information-relay-050-2026-09-26"
	RPSharedPublicAccessSchemaVersion        = "corerp-f7-shared-public-notice-access-051-2026-09-26"
	RPSharedOrganizationAccessSchemaVersion  = "corerp-f7-shared-organization-notice-access-052-2026-09-26"
	RPSharedPublicPublishSchemaVersion       = "corerp-f7-shared-public-notice-publish-053-2026-09-26"
	RPSharedOrganizationPublishSchemaVersion = "corerp-f7-shared-organization-notice-publish-054-2026-09-26"
	OrganizationAgencySchemaVersion          = "corerp-f8-organization-agency-055-2026-09-26"
	OrganizationReviewScheduleSchemaVersion  = "corerp-f8-organization-review-schedule-056-2026-09-26"
	StudioLifeSeedingSchemaVersion           = "corerp-studio-life-seeding-057-2026-09-27"
	RPActivityContinuitySchemaVersion        = "corerp-rp-activity-continuity-058-2026-09-27"
	RPNarrativeFallbackSchemaVersion         = "corerp-rp-narrative-fallback-059-2026-09-27"
	RPTurnActivationSchemaVersion            = "corerp-rp-turn-activation-060-2026-09-27"
	RPSceneObjectSchemaVersion               = "corerp-rp-scene-objects-061-2026-09-27"
	RPBackgroundProgressionSchemaVersion     = "corerp-rp-background-progression-062-2026-09-27"
	RPSessionChapterSchemaVersion            = "corerp-rp-session-chapters-063-2026-09-27"
	RPOfficialNarrativeSchemaVersion         = "corerp-rp-official-narrative-064-2026-09-27"
	RPProviderReceiptsSchemaVersion          = "corerp-rp-provider-receipts-065-2026-09-27"
	RPWaitDerivedSettleSchemaVersion         = "corerp-rp-wait-derived-settle-066-2026-09-27"
	RPTurnExplicitRebaseSchemaVersion        = "corerp-rp-turn-explicit-rebases-067-2026-09-27"
	RPInteractionInterpretationVersion       = "corerp-rp-interaction-interpretations-068-2026-09-27"
	RPWaitSettleLineageSchemaVersion         = "corerp-rp-wait-settle-lineage-069-2026-09-27"
	RPObjectSchemaVersion                    = "corerp-rp-object-interactions-070-2026-09-27"
	RPTypedActionChildSchemaVersion          = "corerp-rp-typed-action-children-071-2026-09-27"
	RPObjectStowSchemaVersion                = "corerp-rp-object-stow-072-2026-09-28"
	RPNarrativeRendersSchemaVersion          = "corerp-rp-narrative-renders-073-2026-09-28"
	RPLifePostingsIndexSchemaVersion         = "corerp-rp-life-postings-index-074-2026-09-28"
	RPAccountEconomicSourcesVersion          = "corerp-rp-account-economic-sources-075-2026-09-28"
	RPConversationFocusSchemaVersion         = "corerp-rp-conversation-focus-076-2026-09-30"
	RPNarrativeCompositionSchemaVersion      = "corerp-rp-narrative-composition-077-2026-10-01"
	RPNarrativeArtifactsSchemaVersion        = "corerp-rp-narrative-artifacts-078-2026-10-01"
	RPSessionOpenedWindowSchemaVersion       = "corerp-rp-session-opened-window-079-2026-10-01"
	SchemaVersion                            = RPSessionOpenedWindowSchemaVersion
	legacyStudioLifeSeedingSchemaVersion     = "corerp-studio-life-seeding-042-2026-09-26"
	legacyRPActivityContinuitySchemaVersion  = "corerp-rp-activity-continuity-043-2026-09-26"
	legacyRPNarrativeFallbackSchemaVersion   = "corerp-rp-narrative-fallback-044-2026-09-26"
)

const (
	DemoInstanceID          = "inst_m1"
	DemoBranchID            = "br_main"
	DemoEpochID             = "epoch_0"
	DemoCurrencyID          = "credit"
	DemoBuyerAccountID      = "account_buyer"
	DemoSellerAccountID     = "account_seller"
	DemoIssuanceAccountID   = "account_issuance"
	DemoEmployerAccountID   = "account_enterprise"
	DemoEmployee2AccountID  = "account_employee_2"
	DemoEmployee3AccountID  = "account_employee_3"
	DemoLandlordAccountID   = "account_landlord"
	DemoBuyerLocationID     = "location_buyer"
	DemoEmployee2LocationID = "location_employee_2"
	DemoEmployee3LocationID = "location_employee_3"
	DemoSellerLocationID    = "location_seller"
	DemoSourceLocationID    = "location_inventory_source"
	DemoSKUID               = "sku_bread"
)

const (
	DemoEnterpriseEntityID = "entity_enterprise"
	DemoEmployee1EntityID  = "entity_employee_1"
	DemoEmployee2EntityID  = "entity_employee_2"
	DemoEmployee3EntityID  = "entity_employee_3"
	DemoLandlordEntityID   = "entity_landlord"
	DemoStoreEntityID      = "entity_store"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	db                       *sql.DB
	now                      func() time.Time
	beforeCommit             func() error
	afterPublish             func(OutboxMessage) error
	afterRPTurnStage         func(string) error
	afterRPSharedActionStage func(string) error
	afterRPInteractionStep   func(int) error
	reverseAgentSeed         bool
}

type State struct {
	HeadSequence    int64 `json:"head_sequence"`
	BuyerBalance    int64 `json:"buyer_balance"`
	SellerBalance   int64 `json:"seller_balance"`
	BuyerInventory  int64 `json:"buyer_inventory"`
	SellerInventory int64 `json:"seller_inventory"`
	Commands        int64 `json:"commands"`
	Events          int64 `json:"events"`
	JournalEntries  int64 `json:"journal_entries"`
	Postings        int64 `json:"postings"`
	StockMovements  int64 `json:"stock_movements"`
	OutboxRows      int64 `json:"outbox_rows"`
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, core.NewError(core.CodeInvalidArgument, "database path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "resolve database path", err)
	}
	params := url.Values{}
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "busy_timeout(5000)")
	params.Add("_pragma", "journal_mode(WAL)")
	location := url.URL{Scheme: "file", Path: filepath.ToSlash(absPath), RawQuery: params.Encode()}
	dsn := location.String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "open SQLite", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, now: time.Now}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, core.WrapError(core.CodeStorageFailure, "ping SQLite", err)
	}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

var schemaMigrations = []struct {
	version  string
	filename string
}{
	{RecoverySchemaVersion, "002_recovery.sql"},
	{StrictSchemaVersion, "003_strict_world.sql"},
	{AccountingSchemaVersion, "004_obligation_accounting.sql"},
	{AuthorizationSchemaVersion, "005_authorization_issuance.sql"},
	{CohortSchemaVersion, "006_cohort_materialization.sql"},
	{AgentSchemaVersion, "007_agent_life.sql"},
	{EconomySchemaVersion, "008_m2_economy.sql"},
	{StoreSchemaVersion, "009_m2_store.sql"},
	{SupplySchemaVersion, "010_m2_consumption_supply.sql"},
	{ArrearsSchemaVersion, "011_m2_arrears.sql"},
	{InsolvencySchemaVersion, "012_m2_insolvency.sql"},
	{ClaimsSchemaVersion, "013_m2_bankruptcy_claims.sql"},
	{AllocationSchemaVersion, "014_m2_claim_allocations.sql"},
	{EstateSchemaVersion, "015_m2_estate_distribution.sql"},
	{WageParticipationSchemaVersion, "016_m2_wage_participation.sql"},
	{WageAllocationSchemaVersion, "017_m2_wage_allocation_policy.sql"},
	{WageClaimOwnershipSchemaVersion, "018_m2_wage_claim_ownership.sql"},
	{BankruptcySlotSchemaVersion, "019_m2_bankruptcy_slot_claims.sql"},
	{RPSessionSchemaVersion, "020_rp_sessions.sql"},
	{RPRouteSchemaVersion, "021_rp_routes.sql"},
	{RPWaitSchemaVersion, "022_rp_wait_intents.sql"},
	{RPSpeechSchemaVersion, "023_rp_utterances.sql"},
	{RPNPCDecisionSchemaVersion, "024_rp_npc_decisions.sql"},
	{RPTurnSchemaVersion, "025_rp_turn_runs.sql"},
	{RPStyleSchemaVersion, "026_rp_styles.sql"},
	{RPRequestSchemaVersion, "027_rp_request_retirement.sql"},
	{StudioPackageSchemaVersion, "028_studio_package_content.sql"},
	{StudioActivationSchemaVersion, "029_studio_package_activation.sql"},
	{StudioReadySchemaVersion, "030_studio_world_ready.sql"},
	{RPInteractionSchemaVersion, "031_rp_interactions.sql"},
	{RPSpatialLocationSchemaVersion, "032_spatial_locations.sql"},
	{RPSpatialJourneySchemaVersion, "033_spatial_journeys.sql"},
	{RPSpatialPerceptionSchemaVersion, "034_spatial_perception.sql"},
	{RPSpatialIdentitySchemaVersion, "035_spatial_identity.sql"},
	{RPControllerEnrollmentSchemaVersion, "036_controller_enrollment.sql"},
	{RPControllerAuthoritySchemaVersion, "037_controller_authority.sql"},
	{RPSharedRoundsSchemaVersion, "038_shared_rounds.sql"},
	{RPControllerLifecycleSchemaVersion, "039_controller_lifecycle.sql"},
	{RPSharedActionSchemaVersion, "040_shared_action_rounds.sql"},
	{RPSharedMoveSchemaVersion, "041_shared_move_rounds.sql"},
	{RPHouseholdSchemaVersion, "042_households.sql"},
	{RPHouseholdRentSchemaVersion, "043_household_rent_agreements.sql"},
	{RPHouseholdContributionSchemaVersion, "044_household_rent_contributions.sql"},
	{RPHouseholdDependentSchemaVersion, "045_household_dependents.sql"},
	{RPSharedHealthSchemaVersion, "046_shared_health_rounds.sql"},
	{RPInformationSchemaVersion, "047_information_channels.sql"},
	{RPSharedInformationSchemaVersion, "048_shared_information_rounds.sql"},
	{RPSharedStanceSchemaVersion, "049_shared_information_stance.sql"},
	{RPSharedRelaySchemaVersion, "050_shared_information_relay.sql"},
	{RPSharedPublicAccessSchemaVersion, "051_shared_public_notice_access.sql"},
	{RPSharedOrganizationAccessSchemaVersion, "052_shared_organization_notice_access.sql"},
	{RPSharedPublicPublishSchemaVersion, "053_shared_public_notice_publish.sql"},
	{RPSharedOrganizationPublishSchemaVersion, "054_shared_organization_notice_publish.sql"},
	{OrganizationAgencySchemaVersion, "055_organization_agency.sql"},
	{OrganizationReviewScheduleSchemaVersion, "056_organization_review_schedule.sql"},
	{StudioLifeSeedingSchemaVersion, "057_studio_life_seeding.sql"},
	{RPActivityContinuitySchemaVersion, "058_rp_activity_continuity.sql"},
	{RPNarrativeFallbackSchemaVersion, "059_rp_narrative_fallback.sql"},
	{RPTurnActivationSchemaVersion, "060_rp_turn_activation.sql"},
	{RPSceneObjectSchemaVersion, "061_rp_scene_objects.sql"},
	{RPBackgroundProgressionSchemaVersion, "062_rp_background_progression.sql"},
	{RPSessionChapterSchemaVersion, "063_rp_session_chapters.sql"},
	{RPOfficialNarrativeSchemaVersion, "064_rp_official_narrative.sql"},
	{RPProviderReceiptsSchemaVersion, "065_rp_provider_receipts.sql"},
	{RPWaitDerivedSettleSchemaVersion, "066_rp_wait_derived_settle.sql"},
	{RPTurnExplicitRebaseSchemaVersion, "067_rp_turn_explicit_rebases.sql"},
	{RPInteractionInterpretationVersion, "068_rp_interaction_interpretations.sql"},
	{RPWaitSettleLineageSchemaVersion, "069_rp_wait_settle_lineage.sql"},
	{RPObjectSchemaVersion, "070_rp_object_interactions.sql"},
	{RPTypedActionChildSchemaVersion, "071_rp_typed_action_children.sql"},
	{RPObjectStowSchemaVersion, "072_rp_object_stow.sql"},
	{RPNarrativeRendersSchemaVersion, "073_rp_narrative_renders.sql"},
	{RPLifePostingsIndexSchemaVersion, "074_rp_life_postings_index.sql"},
	{RPAccountEconomicSourcesVersion, "075_rp_account_economic_sources.sql"},
	{RPConversationFocusSchemaVersion, "076_rp_conversation_focus.sql"},
	{RPNarrativeCompositionSchemaVersion, "077_rp_narrative_composition.sql"},
	{RPNarrativeArtifactsSchemaVersion, "078_rp_narrative_artifacts.sql"},
	{RPSessionOpenedWindowSchemaVersion, "079_rp_session_opened_window.sql"},
}

func (s *Store) migrate(ctx context.Context) error {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_meta'`).Scan(&count)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "inspect schema", err)
	}
	if count == 0 {
		if err := s.applyMigration(ctx, "001_init.sql"); err != nil {
			return err
		}
	}
	hasBase, err := s.hasSchemaVersion(ctx, BaseSchemaVersion)
	if err != nil {
		return err
	}
	if !hasBase {
		return core.NewError(core.CodeStorageFailure, fmt.Sprintf("database is missing required base schema %q", BaseSchemaVersion))
	}
	if err := s.normalizeLegacyEStageMigrations(ctx); err != nil {
		return err
	}
	for _, migration := range schemaMigrations {
		hasVersion, err := s.hasSchemaVersion(ctx, migration.version)
		if err != nil {
			return err
		}
		if !hasVersion {
			if err := s.applyMigration(ctx, migration.filename); err != nil {
				return err
			}
		}
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		return core.WrapError(core.CodeStorageFailure, "foreign key enforcement is not active", err)
	}
	return nil
}

// normalizeLegacyEStageMigrations bridges databases created by the earlier E
// integration worktree, where these three migrations occupied 042-044 before
// F4-F8 claimed those sequence numbers in the merged migration line. The
// legacy schemas are structurally equivalent to 057 and 059. Migration 058
// additionally introduced a historical own-action backfill, so that delta is
// applied idempotently before recording the canonical version.
func (s *Store) normalizeLegacyEStageMigrations(ctx context.Context) error {
	type alias struct {
		legacy   string
		current  string
		backfill string
	}
	aliases := []alias{
		{legacy: legacyStudioLifeSeedingSchemaVersion, current: StudioLifeSeedingSchemaVersion},
		{
			legacy:  legacyRPActivityContinuitySchemaVersion,
			current: RPActivityContinuitySchemaVersion,
			backfill: `
INSERT OR IGNORE INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence)
SELECT e.actor_id,e.event_id,'speech',NULL,json_extract(e.payload,'$.text'),json_extract(e.payload,'$.place_id'),e.world_time,NULL,e.instance_id,e.branch_id,e.event_sequence
FROM events e WHERE e.event_type='RPSpeechAccepted';
INSERT OR IGNORE INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence)
SELECT e.actor_id,e.event_id,'leave',NULL,NULL,json_extract(e.payload,'$.to_place_id'),e.world_time,NULL,e.instance_id,e.branch_id,e.event_sequence
FROM events e WHERE e.event_type='RPNPCMoved';
INSERT OR IGNORE INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence)
SELECT e.actor_id,e.event_id,json_extract(e.payload,'$.action'),NULL,NULL,COALESCE(json_extract(e.payload,'$.from_place_id'),json_extract(e.payload,'$.place_id')),e.world_time,NULL,e.instance_id,e.branch_id,e.event_sequence
FROM events e WHERE e.event_type='RPNPCDecisionRecorded' AND json_extract(e.payload,'$.action') IN ('silence','wait');`,
		},
		{legacy: legacyRPNarrativeFallbackSchemaVersion, current: RPNarrativeFallbackSchemaVersion},
	}

	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin legacy E migration normalization", err)
	}
	for _, item := range aliases {
		var legacyCount, currentCount int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, item.legacy).Scan(&legacyCount); err != nil {
			tx.Rollback(ctx)
			return core.WrapError(core.CodeStorageFailure, "read legacy E migration version", err)
		}
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, item.current).Scan(&currentCount); err != nil {
			tx.Rollback(ctx)
			return core.WrapError(core.CodeStorageFailure, "read canonical E migration version", err)
		}
		if legacyCount == 0 || currentCount != 0 {
			continue
		}
		if item.backfill != "" {
			if _, err := tx.conn.ExecContext(ctx, item.backfill); err != nil {
				tx.Rollback(ctx)
				return core.WrapError(core.CodeStorageFailure, "backfill legacy E activity projection", err)
			}
		}
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO schema_meta(schema_version,applied_at_utc) VALUES (?,?)`, item.current, "2026-09-27T00:00:00Z"); err != nil {
			tx.Rollback(ctx)
			return core.WrapError(core.CodeStorageFailure, "record canonical E migration alias", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit legacy E migration normalization", err)
	}
	return nil
}

func (s *Store) hasSchemaVersion(ctx context.Context, version string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version = ?`, version).Scan(&count); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "read schema version", err)
	}
	return count == 1, nil
}

func (s *Store) applyMigration(ctx context.Context, filename string) error {
	schema, err := migrationFiles.ReadFile("migrations/" + filename)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read embedded migration "+filename, err)
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin migration "+filename, err)
	}
	if _, err := tx.conn.ExecContext(ctx, string(schema)); err != nil {
		tx.Rollback(ctx)
		return core.WrapError(core.CodeStorageFailure, "apply migration "+filename, err)
	}
	if filename == "072_rp_object_stow.sql" {
		rows, err := tx.conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
		if err != nil {
			tx.Rollback(ctx)
			return core.WrapError(core.CodeStorageFailure, "check migrated object references", err)
		}
		broken := rows.Next()
		err = rows.Err()
		rows.Close()
		if err != nil {
			tx.Rollback(ctx)
			return core.WrapError(core.CodeStorageFailure, "check migrated object references", err)
		}
		if broken {
			tx.Rollback(ctx)
			return core.NewError(core.CodeStorageFailure, "object migration has invalid foreign keys")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit migration "+filename, err)
	}
	return nil
}

type immediateTx struct {
	conn   *sql.Conn
	closed bool
}

func beginImmediate(ctx context.Context, db *sql.DB) (*immediateTx, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		conn.Close()
		return nil, err
	}
	return &immediateTx{conn: conn}, nil
}

func (tx *immediateTx) Commit(ctx context.Context) error {
	if tx.closed {
		return errors.New("transaction already closed")
	}
	_, err := tx.conn.ExecContext(ctx, "COMMIT")
	if err != nil {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		_, _ = tx.conn.ExecContext(rollbackCtx, "ROLLBACK")
		cancel()
	}
	tx.closed = true
	closeErr := tx.conn.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (tx *immediateTx) Rollback(ctx context.Context) {
	if !tx.closed {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		_, _ = tx.conn.ExecContext(rollbackCtx, "ROLLBACK")
		cancel()
		tx.closed = true
	}
	_ = tx.conn.Close()
}
