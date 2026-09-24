package httpapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"

	"corerp.local/backend/internal/core"
)

type CursorCodec struct {
	aead cipher.AEAD
}

type cursorDocument struct {
	Version     int    `json:"version"`
	PrincipalID string `json:"principal_id"`
	InstanceID  string `json:"instance_id"`
	BranchID    string `json:"branch_id"`
	Sequence    int64  `json:"sequence"`
	SessionID   string `json:"session_id,omitempty"`
	ObserverID  string `json:"observer_id,omitempty"`
}

func NewCursorCodec(secret []byte) (*CursorCodec, error) {
	if len(secret) < 32 {
		return nil, core.NewError(core.CodeInvalidArgument, "cursor secret must contain at least 32 bytes")
	}
	key := sha256.Sum256(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "initialize cursor cipher", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "initialize cursor authentication", err)
	}
	return &CursorCodec{aead: aead}, nil
}

func (c *CursorCodec) Encode(principalID, instanceID, branchID string, sequence int64) (string, error) {
	return c.encodeDocument(cursorDocument{Version: 1, PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID, Sequence: sequence})
}

// Version 2 is exclusively the RP stream scope. A cursor is a continuation
// token, not authorization: every page must recheck the live session and grant.
func (c *CursorCodec) EncodeRP(principalID, instanceID, branchID, sessionID, observerID string, sequence int64) (string, error) {
	if principalID == "" || instanceID == "" || branchID == "" || sessionID == "" || observerID == "" {
		return "", core.NewError(core.CodeInvalidArgument, "RP cursor requires a complete session scope")
	}
	return c.encodeDocument(cursorDocument{Version: 2, PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID, SessionID: sessionID, ObserverID: observerID, Sequence: sequence})
}

func (c *CursorCodec) encodeDocument(document cursorDocument) (string, error) {
	if document.Sequence < 0 {
		return "", core.NewError(core.CodeInvalidArgument, "cursor sequence must be nonnegative")
	}
	plaintext, err := json.Marshal(document)
	if err != nil {
		return "", core.WrapError(core.CodeStorageFailure, "encode event cursor", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", core.WrapError(core.CodeStorageFailure, "generate event cursor nonce", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *CursorCodec) Decode(encoded, principalID, instanceID, branchID string) (int64, error) {
	document, err := c.decodeDocument(encoded, 1)
	if err != nil {
		return 0, err
	}
	if document.PrincipalID != principalID || document.InstanceID != instanceID || document.BranchID != branchID {
		return 0, core.NewError(core.CodeUnauthorized, "event cursor does not belong to this principal or scope")
	}
	return document.Sequence, nil
}

func (c *CursorCodec) DecodeRP(encoded, principalID, instanceID, branchID, sessionID, observerID string) (int64, error) {
	document, err := c.decodeDocument(encoded, 2)
	if err != nil {
		return 0, err
	}
	if document.SessionID == "" || document.ObserverID == "" || document.PrincipalID == "" || document.InstanceID == "" || document.BranchID == "" {
		return 0, core.NewError(core.CodeInvalidArgument, "RP cursor requires a complete session scope")
	}
	if document.PrincipalID != principalID || document.InstanceID != instanceID || document.BranchID != branchID || document.SessionID != sessionID || document.ObserverID != observerID {
		return 0, core.NewError(core.CodeUnauthorized, "RP cursor does not belong to this principal or session scope")
	}
	return document.Sequence, nil
}

func (c *CursorCodec) decodeDocument(encoded string, version int) (cursorDocument, error) {
	var document cursorDocument
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(sealed) < c.aead.NonceSize() {
		return document, core.NewError(core.CodeInvalidArgument, "event cursor is invalid")
	}
	nonce := sealed[:c.aead.NonceSize()]
	ciphertext := sealed[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return document, core.NewError(core.CodeInvalidArgument, "event cursor is invalid")
	}
	if err := json.Unmarshal(plaintext, &document); err != nil || document.Version != version || document.Sequence < 0 {
		return document, core.NewError(core.CodeInvalidArgument, "event cursor is invalid")
	}
	return document, nil
}
