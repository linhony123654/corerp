package httpapi

import (
	"encoding/base64"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPCursorScopeRecoveryAndProtocolIsolation(t *testing.T) {
	secret := []byte("test-rp-cursor-secret-at-least-32-bytes")
	codec, err := NewCursorCodec(secret)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := codec.EncodeRP("player", "world", "branch", "session", "observer", 42)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewCursorCodec(secret)
	if err != nil {
		t.Fatal(err)
	}
	if sequence, err := reopened.DecodeRP(encoded, "player", "world", "branch", "session", "observer"); err != nil || sequence != 42 {
		t.Fatalf("restart continuation: sequence=%d err=%v", sequence, err)
	}
	for index, field := range []string{"principal", "world", "branch", "session", "observer"} {
		t.Run(field, func(t *testing.T) {
			scope := []string{"player", "world", "branch", "session", "observer"}
			scope[index] += "-other"
			if _, err := codec.DecodeRP(encoded, scope[0], scope[1], scope[2], scope[3], scope[4]); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("cross-%s replay: %v", field, err)
			}
			scope[index] = ""
			if _, err := codec.EncodeRP(scope[0], scope[1], scope[2], scope[3], scope[4], 0); !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatalf("empty %s accepted: %v", field, err)
			}
		})
	}
	legacy, err := codec.Encode("player", "world", "branch", 42)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codec.DecodeRP(legacy, "player", "world", "branch", "session", "observer"); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("legacy cursor accepted by RP: %v", err)
	}
	if _, err := codec.Decode(encoded, "player", "world", "branch"); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("RP cursor accepted by legacy API: %v", err)
	}
	if _, err := codec.EncodeRP("player", "world", "branch", "session", "observer", -1); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("negative sequence accepted: %v", err)
	}
	zero, err := codec.EncodeRP("player", "world", "branch", "session", "observer", 0)
	if err != nil {
		t.Fatal(err)
	}
	if sequence, err := codec.DecodeRP(zero, "player", "world", "branch", "session", "observer"); err != nil || sequence != 0 {
		t.Fatalf("initial cursor: sequence=%d err=%v", sequence, err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	for _, invalid := range []string{"", "!invalid-base64", base64.RawURLEncoding.EncodeToString(raw), encoded[:10]} {
		if _, err := codec.DecodeRP(invalid, "player", "world", "branch", "session", "observer"); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("malformed or tampered cursor accepted: %v", err)
		}
	}
	otherKey, err := NewCursorCodec([]byte("a-different-rp-cursor-secret-of-32-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherKey.DecodeRP(encoded, "player", "world", "branch", "session", "observer"); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("wrong-key cursor accepted: %v", err)
	}
	second, err := codec.EncodeRP("player", "world", "branch", "session", "observer", 42)
	if err != nil || second == encoded {
		t.Fatalf("nonce uniqueness: identical=%v err=%v", second == encoded, err)
	}
}
