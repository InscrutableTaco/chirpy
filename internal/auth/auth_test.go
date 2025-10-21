package auth_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonathangibson/chirpy/internal/auth" // adjust import path
)

const secret = "test-secret"

func TestMakeAndValidateJWT_Succeeds(t *testing.T) {
	userID := uuid.New()
	tok, err := auth.MakeJWT(userID, secret, time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT err: %v", err)
	}
	gotID, err := auth.ValidateJWT(tok, secret)
	if err != nil {
		t.Fatalf("ValidateJWT err: %v", err)
	}
	if gotID != userID {
		t.Fatalf("want %s, got %s", userID, gotID)
	}
}

func TestValidateJWT_Expired(t *testing.T) {
	userID := uuid.New()
	tok, err := auth.MakeJWT(userID, secret, -1*time.Second)
	if err != nil {
		t.Fatalf("MakeJWT err: %v", err)
	}
	if _, err := auth.ValidateJWT(tok, secret); err == nil {
		t.Fatalf("expected error for expired token")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	userID := uuid.New()
	tok, err := auth.MakeJWT(userID, secret, time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT err: %v", err)
	}
	if _, err := auth.ValidateJWT(tok, "bad-secret"); err == nil {
		t.Fatalf("expected error for wrong secret")
	}
}

// go
func TestGetBearerToken(t *testing.T) {
	hdrs := http.Header{}
	hdrs.Set("Authorization", "Bearer abc.def.ghi")
	tok, err := auth.GetBearerToken(hdrs)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if tok != "abc.def.ghi" {
		t.Fatalf("got %q, want %q", tok, "abc.def.ghi")
	}
}

func TestGetBearerToken_MissingHeader(t *testing.T) {
	hdrs := http.Header{}
	_, err := auth.GetBearerToken(hdrs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
