package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"

	"corerp.local/backend/internal/core"
)

func rpIdentityKnown(ctx context.Context, conn *sql.Conn, instance, branch, observer, subject string) (bool, error) {
	if observer == subject {
		return true, nil
	}
	var known int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE instance_id=? AND branch_id=? AND observer_agent_id=? AND subject_agent_id=?`, instance, branch, observer, subject).Scan(&known)
	return known == 1, err
}

// Anonymous handles are stable within one observer/world, but never encode an
// agent's name or authoritative ID. They are presentation IDs, not Event keys.
func rpAnonymousEntityID(ctx context.Context, conn *sql.Conn, instance, branch, observer, subject string) (string, error) {
	return rpAnonymousReference(ctx, conn, "person", instance, branch, observer, subject)
}

func rpAnonymousEvidenceID(ctx context.Context, conn *sql.Conn, instance, branch, observer, source string) (string, error) {
	return rpAnonymousReference(ctx, conn, "evidence", instance, branch, observer, source)
}

func rpAnonymousReference(ctx context.Context, conn *sql.Conn, kind, instance, branch, observer, source string) (string, error) {
	var secret []byte
	if err := conn.QueryRowContext(ctx, `SELECT secret FROM rp_identity_alias_secret WHERE singleton=1`).Scan(&secret); err != nil {
		return "", err
	}
	if len(secret) != 32 {
		return "", core.NewError(core.CodeProjectionDiverged, "invalid identity alias secret")
	}
	encoded, err := core.CanonicalJSON([]string{"rp-anonymous-v1", kind, instance, branch, observer, source})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(encoded)
	return kind + "_" + hex.EncodeToString(mac.Sum(nil)[:12]), nil
}

func rpPublicEntityID(ctx context.Context, conn *sql.Conn, instance, branch, observer, subject string) (string, error) {
	known, err := rpIdentityKnown(ctx, conn, instance, branch, observer, subject)
	if err != nil || known {
		return subject, err
	}
	return rpAnonymousEntityID(ctx, conn, instance, branch, observer, subject)
}

func rpResolvePublicEntityID(ctx context.Context, conn *sql.Conn, instance, branch, observer, supplied string) (string, error) {
	if !strings.HasPrefix(supplied, "person_") {
		return supplied, nil // trusted/local callers may already hold the authoritative ID
	}
	rows, err := conn.QueryContext(ctx, `SELECT agent_id FROM agent_profiles WHERE instance_id=? AND branch_id=? AND status='active'`, instance, branch)
	if err != nil {
		return "", err
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return "", err
		}
		candidates = append(candidates, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	for _, id := range candidates {
		alias, err := rpAnonymousEntityID(ctx, conn, instance, branch, observer, id)
		if err != nil {
			return "", err
		}
		if alias == supplied {
			return id, nil
		}
	}
	return "", core.NewError(core.CodeNotFound, "anonymous person is not in this world")
}
