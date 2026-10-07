package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2Memory (KiB) and argon2Time are the OWASP Password Storage Cheat Sheet minimum for Argon2id.
const (
	argon2Memory  = 19 * 1024
	argon2Time    = 2
	argon2Threads = 1
	argon2SaltLen = 16
	argon2KeyLen  = 32
	phcPrefix     = "$argon2id$v=19$"
	phcFields     = 3
)

var errUnsupportedHash = errors.New("unsupported password hash format")

func hashPassword(password string) string {
	salt := make([]byte, argon2SaltLen)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return fmt.Sprintf(phcPrefix+"m=%d,t=%d,p=%d$%s$%s",
		argon2Memory, argon2Time, argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}

func verifyPassword(encoded, password string) (bool, error) {
	params, ok := strings.CutPrefix(encoded, phcPrefix)
	fields := strings.Split(params, "$")
	if !ok || len(fields) != phcFields {
		return false, errUnsupportedHash
	}

	var memory, iterations uint32
	var threads uint8
	_, err := fmt.Sscanf(fields[0], "m=%d,t=%d,p=%d", &memory, &iterations, &threads)
	if err != nil || iterations < 1 || threads < 1 {
		return false, errUnsupportedHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(fields[1])
	if err != nil {
		return false, errUnsupportedHash
	}

	want, err := base64.RawStdEncoding.DecodeString(fields[2])
	if err != nil || len(want) != argon2KeyLen {
		return false, errUnsupportedHash
	}

	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, argon2KeyLen)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
