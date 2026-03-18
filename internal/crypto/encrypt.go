package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const nonceSizeGCM = 12

var (
	ErrInvalidKey  = errors.New("encryption key must be 32 bytes")
	ErrDecrypt     = errors.New("decryption failed")
)

// Encrypt encrypts plaintext with AES-256-GCM. Key must be 32 bytes.
// Returns base64(nonce || ciphertext) for storage.
func Encrypt(plaintext []byte, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a value produced by Encrypt. If the input is not valid
// base64 or decryption fails (e.g. wrong key or legacy plain text), returns
// (original, nil) so callers can treat as plain text for backward compatibility.
func Decrypt(encoded string, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(ciphertext) < nonceSizeGCM+1 {
		return []byte(encoded), nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return []byte(encoded), nil
	}
	return plain, nil
}

// KeyFromBase64 decodes a base64-encoded 32-byte key. Returns nil if invalid.
func KeyFromBase64(b64 string) []byte {
	if b64 == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(key) != 32 {
		return nil
	}
	return key
}

// KeyFromEnv interpreta ENCRYPTION_KEY: se for base64 com 32 bytes, usa como está;
// senão deriva 32 bytes com SHA-256 da string (permite UUID, senha, etc.). Retorna nil se vazio.
// Remove espaços e quebras de linha para evitar diferença entre API e worker (ex.: .env com CRLF).
func KeyFromEnv(env string) []byte {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil
	}
	if key := KeyFromBase64(env); len(key) == 32 {
		return key
	}
	// Derivar chave de 32 bytes a partir de qualquer string (UUID, senha, etc.)
	h := sha256.Sum256([]byte(env))
	return h[:]
}
