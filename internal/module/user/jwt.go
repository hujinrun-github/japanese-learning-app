package user

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// jwtHeader is the fixed JWT header for HS256.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// jwtClaims holds the JWT payload.
type jwtClaims struct {
	Sub int64 `json:"sub"` // user ID
	Exp int64 `json:"exp"` // Unix timestamp
	Iat int64 `json:"iat"` // issued at
}

// SignToken creates an HS256 JWT for the given userID.
// ttl is the token lifetime (e.g. 24*time.Hour). Negative ttl creates an already-expired token.
func SignToken(userID int64, secret string, ttl time.Duration) (token string, expiresAt time.Time, err error) {
	now := time.Now()
	expiresAt = now.Add(ttl)

	claims := jwtClaims{
		Sub: userID,
		Exp: expiresAt.Unix(),
		Iat: now.Unix(),
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("user.SignToken marshal claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	signingInput := jwtHeader + "." + payload
	sig := sign(signingInput, secret)

	token = signingInput + "." + sig
	return token, expiresAt, nil
}

// VerifyToken parses and validates an HS256 JWT, returning the userID on success.
func VerifyToken(token, secret string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("user.VerifyToken: invalid token format")
	}

	signingInput := parts[0] + "." + parts[1]
	expected := sign(signingInput, secret)
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return 0, fmt.Errorf("user.VerifyToken: invalid signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, fmt.Errorf("user.VerifyToken decode payload: %w", err)
	}

	var claims jwtClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return 0, fmt.Errorf("user.VerifyToken unmarshal claims: %w", err)
	}

	if time.Now().Unix() > claims.Exp {
		return 0, fmt.Errorf("user.VerifyToken: token expired")
	}

	return claims.Sub, nil
}

// sign returns the base64url-encoded HMAC-SHA256 of the input.
func sign(input, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
