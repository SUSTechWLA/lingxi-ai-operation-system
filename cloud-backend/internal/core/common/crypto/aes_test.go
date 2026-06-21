package crypto

import (
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := DeriveKey("test-secret-phrase-for-encryption")
	plaintext := "sk-deepseek-this-is-a-secret-api-key"

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if ciphertext == plaintext {
		t.Fatal("ciphertext should not equal plaintext")
	}
	if len(ciphertext) == 0 {
		t.Fatal("ciphertext should not be empty")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("round-trip failed: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key := DeriveKey("test-secret")
	plaintext := "same-plaintext"

	c1, _ := Encrypt(plaintext, key)
	c2, _ := Encrypt(plaintext, key)

	if c1 == c2 {
		t.Fatal("same plaintext should produce different ciphertexts due to random nonce")
	}

	// Both should decrypt correctly
	d1, _ := Decrypt(c1, key)
	d2, _ := Decrypt(c2, key)
	if d1 != plaintext || d2 != plaintext {
		t.Fatal("different ciphertexts should both decrypt to the same plaintext")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := DeriveKey("secret-one")
	key2 := DeriveKey("secret-two")
	plaintext := "my-api-key"

	ciphertext, _ := Encrypt(plaintext, key1)
	_, err := Decrypt(ciphertext, key2)
	if err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestDecryptTampered(t *testing.T) {
	key := DeriveKey("test-secret")
	ciphertext, _ := Encrypt("secret", key)

	// Tamper with the last byte
	tampered := ciphertext[:len(ciphertext)-1] + "A"
	_, err := Decrypt(tampered, key)
	if err == nil {
		t.Fatal("decrypt of tampered ciphertext should fail (GCM auth)")
	}
}

func TestEncryptInvalidKeySize(t *testing.T) {
	_, err := Encrypt("test", make([]byte, 16))
	if err == nil {
		t.Fatal("should reject 16-byte key")
	}
}

func TestDeriveKeyDeterministic(t *testing.T) {
	k1 := DeriveKey("same-secret")
	k2 := DeriveKey("same-secret")
	if string(k1) != string(k2) {
		t.Fatal("DeriveKey should be deterministic for the same secret")
	}
	if len(k1) != 32 {
		t.Fatalf("DeriveKey should return 32 bytes, got %d", len(k1))
	}
}
