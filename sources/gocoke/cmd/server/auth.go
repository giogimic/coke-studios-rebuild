package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnauthorized        = errors.New("unauthorized: missing or invalid session")
	ErrInvalidCredentials  = errors.New("invalid screen name or password")
	ErrDenizenExists       = errors.New("screen name is already registered")
	ErrSessionExpired      = errors.New("session has expired")
)

const (
	pbkdf2Iterations = 100000
	pbkdf2SaltBytes  = 16
	pbkdf2KeyLen     = 32
	defaultSessionTTL = 24 * time.Hour
)

// HashPassword hashes a plain-text password using PBKDF2-HMAC-SHA256 with a random salt.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}
	salt := make([]byte, pbkdf2SaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	dk := pbkdf2SHA256([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLen)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s",
		pbkdf2Iterations,
		hex.EncodeToString(salt),
		hex.EncodeToString(dk),
	), nil
}

// VerifyPassword checks whether a password matches the stored PBKDF2 hash.
func VerifyPassword(password, encodedHash string) bool {
	if password == "" || encodedHash == "" {
		return false
	}
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expectedDK, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}

	actualDK := pbkdf2SHA256([]byte(password), salt, iter, len(expectedDK))
	return subtle.ConstantTimeCompare(actualDK, expectedDK) == 1
}

// pbkdf2SHA256 implements RFC 2898 / RFC 8018 PBKDF2 with SHA-256.
func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	numBlocks := (keyLen + sha256.Size - 1) / sha256.Size
	var result []byte

	for block := 1; block <= numBlocks; block++ {
		// U_1 = PRF(P, S || INT(i))
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		var blockBytes [4]byte
		blockBytes[0] = byte(block >> 24)
		blockBytes[1] = byte(block >> 16)
		blockBytes[2] = byte(block >> 8)
		blockBytes[3] = byte(block)
		mac.Write(blockBytes[:])
		u := mac.Sum(nil)

		// U_2 ... U_c
		xorSum := make([]byte, len(u))
		copy(xorSum, u)

		for i := 2; i <= iterations; i++ {
			mac.Reset()
			mac.Write(u)
			u = mac.Sum(nil)
			for j := 0; j < len(xorSum); j++ {
				xorSum[j] ^= u[j]
			}
		}

		result = append(result, xorSum...)
	}

	return result[:keyLen]
}

// GenerateSessionToken creates a cryptographically secure 256-bit token.
func GenerateSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// authenticatedDenizen extracts and validates the caller's identity from an HTTP request.
// It checks Bearer tokens, cookies, sid/sw2 query params, and form values.
func (s *Server) authenticatedDenizen(r *http.Request) (*DenizenRecord, error) {
	token := extractRequestToken(r)
	if token == "" {
		return nil, ErrUnauthorized
	}

	// 1. Check sessions table
	sess, err := s.store.GetSession(token)
	if err == nil && sess != nil {
		if time.Now().After(sess.ExpiresAt) {
			_ = s.store.DeleteSession(token)
			return nil, ErrSessionExpired
		}
		denizen, err := s.store.GetDenizenByID(sess.DenizenID)
		if err == nil && denizen != nil {
			return denizen, nil
		}
	}

	// 2. Check permanent denizen secret (for backwards-compatibility with internal Director tokens)
	denizen, err := s.store.GetDenizenBySecret(token)
	if err == nil && denizen != nil {
		return denizen, nil
	}

	return nil, ErrUnauthorized
}

// authenticateRealtime validates realtime socket credentials.
// The secret must match an active unexpired session token or the denizen's permanent secret.
func (s *Server) authenticateRealtime(screenName, secret string) (*DenizenRecord, error) {
	cleanName := strings.TrimSpace(screenName)
	cleanSecret := strings.TrimSpace(secret)
	if cleanName == "" || cleanSecret == "" {
		return nil, ErrInvalidCredentials
	}

	// 1. Verify via active session
	sess, err := s.store.GetSession(cleanSecret)
	if err == nil && sess != nil {
		if time.Now().After(sess.ExpiresAt) {
			_ = s.store.DeleteSession(cleanSecret)
			return nil, ErrSessionExpired
		}
		if !strings.EqualFold(sess.ScreenName, cleanName) {
			// Identity forgery: session belongs to someone else!
			return nil, ErrInvalidCredentials
		}
		denizen, err := s.store.GetDenizenByID(sess.DenizenID)
		if err == nil && denizen != nil {
			return denizen, nil
		}
	}

	// 2. Verify via permanent secret in denizens table
	denizen, err := s.store.GetDenizenByName(cleanName)
	if err == nil && denizen != nil {
		if subtle.ConstantTimeCompare([]byte(denizen.Secret), []byte(cleanSecret)) == 1 {
			return denizen, nil
		}
	}

	return nil, ErrInvalidCredentials
}

// extractRequestToken parses session credentials from standard HTTP locations.
func extractRequestToken(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}

	// 2. Query parameters
	q := r.URL.Query()
	for _, key := range []string{"sid", "sw2", "token", "secret", "sidempty"} {
		if val := q.Get(key); val != "" {
			return strings.TrimSpace(val)
		}
	}

	// 3. Cookies
	if cookie, err := r.Cookie("coke_session"); err == nil && cookie.Value != "" {
		return strings.TrimSpace(cookie.Value)
	}
	if cookie, err := r.Cookie("sid"); err == nil && cookie.Value != "" {
		return strings.TrimSpace(cookie.Value)
	}

	// 4. Form values
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		for _, key := range []string{"sid", "sw2", "token", "secret"} {
			if val := r.FormValue(key); val != "" {
				return strings.TrimSpace(val)
			}
		}
	}

	return ""
}
