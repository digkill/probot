package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// Seal encrypts a value with AES-256-GCM, binding it to a storage location.
func Seal(key, plain, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("invalid encryption key length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1+gcm.NonceSize())
	out[0] = 1
	if _, err := rand.Read(out[1:]); err != nil {
		return nil, err
	}
	return gcm.Seal(out, out[1:], plain, aad), nil
}

func Open(key, sealed, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("invalid encryption key length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < 1+gcm.NonceSize()+gcm.Overhead() || sealed[0] != 1 {
		return nil, errors.New("invalid encrypted value")
	}
	return gcm.Open(nil, sealed[1:1+gcm.NonceSize()], sealed[1+gcm.NonceSize():], aad)
}
