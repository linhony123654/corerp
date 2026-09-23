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
	if sequence < 0 {
		return "", core.NewError(core.CodeInvalidArgument, "cursor sequence must be nonnegative")
	}
	plaintext, err := json.Marshal(cursorDocument{1, principalID, instanceID, branchID, sequence})
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
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(sealed) < c.aead.NonceSize() {
		return 0, core.NewError(core.CodeInvalidArgument, "event cursor is invalid")
	}
	nonce := sealed[:c.aead.NonceSize()]
	ciphertext := sealed[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, core.NewError(core.CodeInvalidArgument, "event cursor is invalid")
	}
	var document cursorDocument
	if err := json.Unmarshal(plaintext, &document); err != nil || document.Version != 1 || document.Sequence < 0 {
		return 0, core.NewError(core.CodeInvalidArgument, "event cursor is invalid")
	}
	if document.PrincipalID != principalID || document.InstanceID != instanceID || document.BranchID != branchID {
		return 0, core.NewError(core.CodeUnauthorized, "event cursor does not belong to this principal or scope")
	}
	return document.Sequence, nil
}
