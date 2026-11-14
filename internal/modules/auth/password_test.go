package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hasher := NewPasswordHasher()

	encoded, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("unexpected hash format: %s", encoded)
	}
	if !hasher.Verify("correct horse battery staple", encoded) {
		t.Error("Verify rejected the correct password")
	}
	if hasher.Verify("wrong password", encoded) {
		t.Error("Verify accepted the wrong password")
	}
}

func TestPasswordHashesUseUniqueSalts(t *testing.T) {
	hasher := NewPasswordHasher()

	first, err := hasher.Hash("same password")
	if err != nil {
		t.Fatalf("first Hash: %v", err)
	}
	second, err := hasher.Hash("same password")
	if err != nil {
		t.Fatalf("second Hash: %v", err)
	}
	if first == second {
		t.Error("two hashes of the same password are identical; salt is not random")
	}
}

func TestPasswordVerifyRejectsMalformedHashes(t *testing.T) {
	hasher := NewPasswordHasher()

	malformed := []string{
		"",
		"plain-text",
		"$argon2i$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=19456$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=not-a-number,t=2,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=1$%%%$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$%%%",
		"$argon2id$v=19$m=999999999,t=2,p=1$c2FsdA$aGFzaA",
	}

	for _, encoded := range malformed {
		if hasher.Verify("password", encoded) {
			t.Errorf("Verify accepted malformed hash %q", encoded)
		}
	}
}

func TestVerifyDummyDoesNotPanic(t *testing.T) {
	NewPasswordHasher().VerifyDummy("anything")
}
