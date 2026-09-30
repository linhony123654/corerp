package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectSourceAndStageRequireDiscreteUnitSKU(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-discrete.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	sourceID, anchorID, _ := objectTestSource(t, ctx, s)
	for _, tc := range []struct {
		name, unit string
		scale      int
	}{
		{"fractional", "unit", 1},
		{"nondiscrete", "kg", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.db.ExecContext(ctx, `UPDATE product_skus SET base_unit=?,quantity_scale=? WHERE sku_id=?`, tc.unit, tc.scale, M2DemoSKUID); err != nil {
				t.Fatal(err)
			}
			_, err := s.DefineRPObjectSource(ctx, RPObjectSourceRequest{
				Binding: careerTestBinding(t, s, "principal_creator", "source-"+tc.name),
				AnchorID: anchorID, OwnerEntityID: M2RPPlayerID, SKUID: M2DemoSKUID, DisplayName: M2DemoSKUID,
			})
			if !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatalf("aggregate SKU declared as a physical source: %v", err)
			}
			stage := objectTestRequest(t, ctx, s, player, "stage", "stage-"+tc.name)
			stage.SourceID = sourceID
			if _, err := s.ObjectRP(ctx, stage); !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatalf("changed SKU definition staged a physical object: %v", err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects`, nil, 0)
			if _, err := s.db.ExecContext(ctx, `UPDATE product_skus SET base_unit='unit',quantity_scale=0 WHERE sku_id=?`, M2DemoSKUID); err != nil {
				t.Fatal(err)
			}
		})
	}
}
