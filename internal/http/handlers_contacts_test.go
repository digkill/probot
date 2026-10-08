package httpapi

import (
	"testing"
	"time"

	"github.com/digkill/probot/internal/config"
	"github.com/google/uuid"
)

func TestExportTokenIsSignedAndExpires(t *testing.T) {
	s := &Server{cfg: &config.Config{JWTSecret: "secret"}}
	ws := uuid.New()
	now := time.Now()
	tok := s.exportToken(ws, now.Add(time.Minute))
	if got, ok := s.verifyExportToken(tok, now); !ok || got != ws {
		t.Fatalf("valid token rejected: %v %v", got, ok)
	}
	if _, ok := s.verifyExportToken(tok, now.Add(2*time.Minute)); ok {
		t.Fatal("expired token accepted")
	}
	other := &Server{cfg: &config.Config{JWTSecret: "other"}}
	if _, ok := other.verifyExportToken(tok, now); ok {
		t.Fatal("token accepted with another secret")
	}
	b := []byte(tok)
	b[3] ^= 1
	if _, ok := s.verifyExportToken(string(b), now); ok {
		t.Fatal("tampered token accepted")
	}
}
