package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. These follow the OWASP minimum recommendation and
// bound the memory used per authentication request.
const (
	argonMemory  uint32 = 19 * 1024 // 19 MiB
	argonTime    uint32 = 2
	argonThreads uint8  = 1
	argonSaltLen        = 16
	argonKeyLen  uint32 = 32
)

// PasswordHasher hashes and verifies passwords with Argon2id.
type PasswordHasher struct {
	dummyOnce sync.Once
	dummyHash string
}

// NewPasswordHasher creates a hasher.
func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{}
}

// Hash returns a PHC-formatted Argon2id hash.
func (h *PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// Verify reports whether password matches the encoded hash. Malformed or
// unsupported hashes return false.
func (h *PasswordHasher) Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}

	var (
		memory  uint32
		timeVal uint32
		threads uint8
	)
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeVal, &threads); err != nil {
		return false
	}
	if memory == 0 || memory > 1<<20 || timeVal == 0 || timeVal > 10 || threads == 0 || threads > 32 {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}

	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false
	}

	actual := argon2.IDKey([]byte(password), salt, timeVal, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// VerifyDummy spends comparable time when the account does not exist, so
// unknown and known emails cannot be distinguished by latency.
func (h *PasswordHasher) VerifyDummy(password string) {
	h.dummyOnce.Do(func() {
		h.dummyHash, _ = h.Hash("sela-timing-equalization-placeholder")
	})
	if h.dummyHash != "" {
		_ = h.Verify(password, h.dummyHash)
	}
}
