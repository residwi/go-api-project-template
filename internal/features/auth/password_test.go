package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	argon2cffiHash        = "$argon2id$v=19$m=64,t=1,p=4$0zo1s47tKoc0BAfwNIPjcA$kgF0pMCpxtOqI1aNqFdjEqx8zkvpaFpxUk0OH3UzKHw"
	argon2cffiLongKeyHash = "$argon2id$v=19$m=64,t=1,p=1$NCQWez0p6+3DC3PWcUk+tg$ZzAl9PdBfi0HdMYO1p9wMs1X5cpECnyR1T8j6UsF7ufanzCrGIXrJQba3nTnlZf8YTsDhKhtwG042m+USz9lxA"
)

func TestHashPassword(t *testing.T) {
	t.Parallel()

	t.Run("encodes a PHC string with the OWASP minimum parameters", func(t *testing.T) {
		t.Parallel()

		assert.Regexp(t,
			`^\$argon2id\$v=19\$m=19456,t=2,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`,
			hashPassword("correct-horse"))
	})

	t.Run("salts every hash uniquely", func(t *testing.T) {
		t.Parallel()

		first := hashPassword("correct-horse")
		second := hashPassword("correct-horse")

		assert.NotEqual(t, first, second)
	})
}

func TestVerifyPassword(t *testing.T) {
	t.Parallel()

	t.Run("accepts the password it was hashed from", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(hashPassword("correct-horse"), "correct-horse")

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("rejects a different password", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(hashPassword("correct-horse"), "battery-staple")

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("accepts a hash from an independent implementation with other parameters", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(argon2cffiHash, "correct-horse")

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("rejects a wrong password against an independent implementation's hash", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(argon2cffiHash, "battery-staple")

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("a bcrypt hash is unsupported", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword("$2a$10$di3MUSPKPZiSdwcCVhRHtu09ZFeGfW29Ag6g6vlO65M7.rxNHOs5a", "admin123456")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("a non-PHC value is unsupported", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword("x", "correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("an empty value is unsupported", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword("", "correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("an Argon2i hash is unsupported", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(
			"$argon2i$v=19$m=64,t=1,p=1$mnuknUkrFeb6uQoKD9Aw8w$9DzDY01O1TzZaSjWhIr7EPXXKUj1mSciB4Vkimxkmv8",
			"correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("an Argon2 version other than 19 is unsupported", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(
			"$argon2id$v=16$m=64,t=1,p=1$mnuknUkrFeb6uQoKD9Aw8w$9DzDY01O1TzZaSjWhIr7EPXXKUj1mSciB4Vkimxkmv8",
			"correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("a non-canonical version field is unsupported", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(
			"$argon2id$v=019$m=64,t=1,p=1$mnuknUkrFeb6uQoKD9Aw8w$9DzDY01O1TzZaSjWhIr7EPXXKUj1mSciB4Vkimxkmv8",
			"correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("zero iterations is unsupported rather than a panic", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(
			"$argon2id$v=19$m=64,t=0,p=1$mnuknUkrFeb6uQoKD9Aw8w$9DzDY01O1TzZaSjWhIr7EPXXKUj1mSciB4Vkimxkmv8",
			"correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("zero parallelism is unsupported rather than a panic", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(
			"$argon2id$v=19$m=64,t=1,p=0$mnuknUkrFeb6uQoKD9Aw8w$9DzDY01O1TzZaSjWhIr7EPXXKUj1mSciB4Vkimxkmv8",
			"correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("a salt that is not base64 is unsupported", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(
			"$argon2id$v=19$m=64,t=1,p=1$!!!$9DzDY01O1TzZaSjWhIr7EPXXKUj1mSciB4Vkimxkmv8",
			"correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("a key other than 32 bytes is unsupported", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(argon2cffiLongKeyHash, "correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("a memory cost above 256 MiB is unsupported rather than an OOM", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(
			"$argon2id$v=19$m=262145,t=1,p=1$mnuknUkrFeb6uQoKD9Aw8w$9DzDY01O1TzZaSjWhIr7EPXXKUj1mSciB4Vkimxkmv8",
			"correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})

	t.Run("more than 10 iterations is unsupported rather than a stalled slot", func(t *testing.T) {
		t.Parallel()

		ok, err := verifyPassword(
			"$argon2id$v=19$m=64,t=11,p=1$mnuknUkrFeb6uQoKD9Aw8w$9DzDY01O1TzZaSjWhIr7EPXXKUj1mSciB4Vkimxkmv8",
			"correct-horse")

		require.ErrorIs(t, err, errUnsupportedHash)
		assert.False(t, ok)
	})
}
