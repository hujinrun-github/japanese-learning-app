package user_test

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
)

func TestSignAndVerifyToken(t *testing.T) {
	secret := "test-secret"
	userID := int64(42)
	ttl := time.Hour

	token, expiresAt, err := user.SignToken(userID, secret, ttl)
	if err != nil {
		t.Fatalf("SignToken error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if expiresAt.Before(time.Now()) {
		t.Errorf("expiresAt should be in the future, got %v", expiresAt)
	}

	gotID, err := user.VerifyToken(token, secret)
	if err != nil {
		t.Fatalf("VerifyToken error: %v", err)
	}
	if gotID != userID {
		t.Errorf("expected userID=%d, got %d", userID, gotID)
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	token, _, err := user.SignToken(1, "correct-secret", time.Hour)
	if err != nil {
		t.Fatalf("SignToken error: %v", err)
	}

	_, err = user.VerifyToken(token, "wrong-secret")
	if err == nil {
		t.Error("expected error with wrong secret")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	// Sign a token that expired 1 second ago.
	token, _, err := user.SignToken(1, "secret", -time.Second)
	if err != nil {
		t.Fatalf("SignToken error: %v", err)
	}

	_, err = user.VerifyToken(token, "secret")
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestVerifyToken_Tampered(t *testing.T) {
	token, _, err := user.SignToken(1, "secret", time.Hour)
	if err != nil {
		t.Fatalf("SignToken error: %v", err)
	}

	// Flip last byte of the token string to tamper with the signature.
	tampered := []byte(token)
	tampered[len(tampered)-1] ^= 0xFF
	_, err = user.VerifyToken(string(tampered), "secret")
	if err == nil {
		t.Error("expected error for tampered token")
	}
}
