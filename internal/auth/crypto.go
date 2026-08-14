package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"

	"github.com/digkill/probot/internal/platforms"
)

func EncryptJSON(key []byte, v any) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func DecryptCredentials(key, blob []byte) (platforms.Credentials, error) {
	var creds platforms.Credentials
	if len(blob) == 0 {
		return creds, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return creds, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return creds, err
	}
	if len(blob) < gcm.NonceSize() {
		return creds, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return creds, err
	}
	if err := json.Unmarshal(plain, &creds); err != nil {
		return creds, err
	}
	return creds, nil
}
