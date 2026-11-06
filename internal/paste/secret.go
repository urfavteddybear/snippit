package paste

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	idLength   = 10
	idAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

	// scrypt cost: 16 MiB of memory per derivation.
	scryptN = 1 << 14
	scryptR = 8
	scryptP = 1
	saltLen = 16
	keyLen  = 32
)

// kdfSlots bounds concurrent scrypt derivations so unlock bursts cannot exhaust memory.
var kdfSlots = make(chan struct{}, 4)

// NewID returns a random base62 ID (~59 bits of entropy).
func NewID() (string, error) {
	// Rejection sampling keeps the distribution uniform: 248 = 4*62.
	out := make([]byte, 0, idLength)
	buf := make([]byte, idLength*2)
	for len(out) < idLength {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b < 248 {
				out = append(out, idAlphabet[int(b)%len(idAlphabet)])
				if len(out) == idLength {
					break
				}
			}
		}
	}
	return string(out), nil
}

// ValidID reports whether s has the shape of a paste ID.
func ValidID(s string) bool {
	if len(s) != idLength {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// newDeletionToken returns a token for the creator and the hash to store.
// The token is high-entropy, so a plain SHA-256 is sufficient.
func newDeletionToken() (token, hash string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenMatches(token, hash string) bool {
	if token == "" || hash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(hash)) == 1
}

// hashDeleteKey stores a user-chosen key as "scrypt$<salt>$<hash>".
// Keys may be weak, so they get the same slow KDF as passwords.
func hashDeleteKey(key string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum, err := deriveKey(key, salt)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return "scrypt$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(sum), nil
}

func deleteKeyMatches(key, stored string) bool {
	if key == "" || stored == "" || len(key) > MaxPasswordBytes {
		return false
	}
	parts := strings.Split(stored, "$")
	if len(parts) != 3 || parts[0] != "scrypt" {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[1])
	want, err2 := enc.DecodeString(parts[2])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := deriveKey(key, salt)
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func deriveKey(password string, salt []byte) ([]byte, error) {
	kdfSlots <- struct{}{}
	defer func() { <-kdfSlots }()
	return scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
}

// seal encrypts plaintext with a key derived from password.
// It returns the salt and nonce||ciphertext.
func seal(password string, plaintext []byte) (salt, sealed []byte, err error) {
	salt = make([]byte, saltLen)
	if _, err = rand.Read(salt); err != nil {
		return nil, nil, err
	}
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return salt, gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func open(password string, salt, sealed []byte) ([]byte, error) {
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("sealed content truncated")
	}
	nonce, ct := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrInvalidPassword
	}
	return pt, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
