package observability

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

const strongPersistentSealingKey = "base64:YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXpBQkNERUY="

func TestParsePersistentSealingKeyEnforcesCanonicalStrongFormat(t *testing.T) {
	overlong := make([]byte, maxPersistentSealingKeyBytes+1)
	for index := range overlong {
		overlong[index] = byte(index)
	}
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "valid strong key", value: strongPersistentSealingKey, valid: true},
		{name: "plain 32 characters", value: strings.Repeat("a", 32)},
		{name: "invalid base64", value: "base64:not/base64%%%"},
		{name: "surrounding whitespace", value: " " + strongPersistentSealingKey},
		{name: "repeated character", value: "base64:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, minPersistentSealingKeyBytes))},
		{name: "repeated block", value: "base64:YWJjZGFiY2RhYmNkYWJjZGFiY2RhYmNkYWJjZGFiY2Q="},
		{name: "repeated hex text", value: "base64:MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="},
		{name: "too short", value: "base64:" + base64.StdEncoding.EncodeToString([]byte("short"))},
		{name: "too long", value: "base64:" + base64.StdEncoding.EncodeToString(overlong)},
		{name: "placeholder", value: "replace-with-a-long-random-observability-sealing-key"},
		{name: "encoded placeholder", value: "base64:" + base64.StdEncoding.EncodeToString([]byte("replace-with-a-long-random-observability-sealing-key"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded, err := ParsePersistentSealingKey(test.value)
			if test.valid {
				if err != nil || len(decoded) != minPersistentSealingKeyBytes {
					t.Fatalf("valid key decoded bytes=%d error=%v", len(decoded), err)
				}
				return
			}
			if err == nil || len(decoded) != 0 {
				t.Fatalf("invalid key decoded bytes=%d error=%v", len(decoded), err)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatalf("validation error leaked supplied key: %v", err)
			}
		})
	}
}

func TestPersistentSealerUsesTheSharedKeyParser(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "plain", value: strings.Repeat("a", 32)},
		{name: "repeated", value: "base64:YWJjZGFiY2RhYmNkYWJjZGFiY2RhYmNkYWJjZGFiY2Q="},
		{name: "strong", value: strongPersistentSealingKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, parseErr := ParsePersistentSealingKey(test.value)
			emitter, sealerErr := NewPersistentEmitter(testSource(), Runtime{}, nil, 1, PersistentSealingConfig{
				Domain: testPersistentDomain,
				Key:    test.value,
			})
			if emitter != nil {
				defer emitter.Close(context.Background())
			}
			if (parseErr == nil) != (sealerErr == nil) {
				t.Fatalf("parser error=%v sealer error=%v", parseErr, sealerErr)
			}
		})
	}
}
