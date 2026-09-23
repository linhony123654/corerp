package httpapi

import (
	"encoding/base64"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCursorIsOpaqueAuthenticatedAndScopeBound(t *testing.T) {
	if _, err := NewCursorCodec([]byte("too-short")); err == nil {
		t.Fatal("short cursor secret should be rejected")
	}
	codec, err := NewCursorCodec([]byte("test-cursor-secret-must-be-at-least-32-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := codec.Encode("principal_buyer", "inst_m1", "br_main", 42)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "principal_buyer") || strings.Contains(string(raw), "inst_m1") || strings.Contains(string(raw), "42") {
		t.Fatalf("cursor exposes plaintext scope or sequence: %q", raw)
	}
	sequence, err := codec.Decode(encoded, "principal_buyer", "inst_m1", "br_main")
	if err != nil || sequence != 42 {
		t.Fatalf("cursor roundtrip mismatch: sequence=%d err=%v", sequence, err)
	}
	if _, err := codec.Decode(encoded, "principal_creator", "inst_m1", "br_main"); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("cross-principal cursor should be denied, got %v", err)
	}
	tampered := []byte(encoded)
	if tampered[len(tampered)/2] == 'A' {
		tampered[len(tampered)/2] = 'B'
	} else {
		tampered[len(tampered)/2] = 'A'
	}
	if _, err := codec.Decode(string(tampered), "principal_buyer", "inst_m1", "br_main"); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("tampered cursor should be rejected, got %v", err)
	}
	second, err := codec.Encode("principal_buyer", "inst_m1", "br_main", 42)
	if err != nil {
		t.Fatal(err)
	}
	if second == encoded {
		t.Fatal("cursor nonce reuse made identical plaintext deterministic")
	}
}
