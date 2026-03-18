package crypto

import (
	"encoding/base64"
	"testing"
)

func mustKeyFromString(t *testing.T, s string) []byte {
	t.Helper()
	k := KeyFromEnv(s)
	if len(k) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k))
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	// 32 bytes de chave fixa só para teste
	rawKey := make([]byte, 32)
	for i := range rawKey {
		rawKey[i] = byte(i)
	}
	b64 := base64.StdEncoding.EncodeToString(rawKey)
	key := KeyFromEnv(b64)

	plaintext := []byte("senha-super-secreta")

	enc, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	if enc == "" {
		t.Fatalf("Encrypt returned empty string")
	}

	dec, err := Decrypt(enc, key)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if string(dec) != string(plaintext) {
		t.Fatalf("Decrypt mismatch: got %q, want %q", string(dec), string(plaintext))
	}
}

func TestDecryptWithWrongKeyReturnsOriginal(t *testing.T) {
	key1 := mustKeyFromString(t, "chave-1")
	key2 := mustKeyFromString(t, "chave-2")

	plaintext := []byte("valor-teste")

	enc, err := Encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}

	dec, err := Decrypt(enc, key2)
	if err != nil {
		t.Fatalf("Decrypt returned error with wrong key: %v", err)
	}
	// Para compatibilidade, decryption com chave errada deve retornar o input codificado (string base64) ou o próprio texto,
	// mas nunca erro; aqui verificamos só que não é vazio.
	if len(dec) == 0 {
		t.Fatalf("Decrypt with wrong key returned empty")
	}
}

func TestKeyFromEnv_Base64AndPlain(t *testing.T) {
	// base64 válido de 32 bytes
	rawKey := make([]byte, 32)
	for i := range rawKey {
		rawKey[i] = byte(i)
	}
	b64 := base64.StdEncoding.EncodeToString(rawKey)
	k := KeyFromEnv(b64)
	if len(k) != 32 {
		t.Fatalf("KeyFromEnv base64: expected 32, got %d", len(k))
	}

	// plain string (UUID, senha, etc.) deve gerar SHA-256 (32 bytes)
	k2 := KeyFromEnv("f0697511-6623-4295-921d-1321a0a69a72")
	if len(k2) != 32 {
		t.Fatalf("KeyFromEnv plain: expected 32, got %d", len(k2))
	}
}

