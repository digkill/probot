package mail

import (
	"context"
	"encoding/base64"
	"net/smtp"
	"strings"
	"testing"
	"time"
)

func TestBuildMessageRejectsHeaderInjection(t *testing.T) {
	for _, to := range []string{"a@b.c\r\nBcc: x@evil.test", "not-an-address", ""} {
		if _, err := buildMessage("PRobot <no-reply@prbo.ru>", to, "s", "b", time.Now()); err == nil {
			t.Fatalf("accepted recipient %q", to)
		}
	}
}

func TestBuildMessageEncodesSubjectAndBody(t *testing.T) {
	msg, err := buildMessage("PRobot <no-reply@prbo.ru>", "user@example.com", "Сброс пароля", "Ссылка: https://prbo.ru/reset-password?token=abc", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s := string(msg)
	if strings.Contains(s, "Сброс") {
		t.Fatal("subject not MIME-encoded")
	}
	_, body, _ := strings.Cut(s, "\r\n\r\n")
	dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", ""))
	if err != nil || !strings.Contains(string(dec), "token=abc") {
		t.Fatalf("body not round-tripped: %v", err)
	}
}

func TestSMTPSenderValidatesBeforeDialing(t *testing.T) {
	s := New(Config{Host: "127.0.0.1", Port: 1, From: "no-reply@prbo.ru"}, nil)
	err := s.Send(context.Background(), "x@y.z\nBcc: a@b.c", "s", "b")
	if err == nil || strings.Contains(err.Error(), "dial") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestPickAuthFallsBackToLogin(t *testing.T) {
	cfg := Config{Host: "smtp.example.com", User: "u@example.com", Password: "p"}
	if a, err := pickAuth("LOGIN", cfg); err != nil {
		t.Fatal(err)
	} else if _, ok := a.(loginAuth); !ok {
		t.Fatalf("LOGIN-only server got %T", a)
	}
	if a, _ := pickAuth("LOGIN PLAIN CRAM-MD5", cfg); a == nil {
		t.Fatal("no auth for PLAIN server")
	} else if _, ok := a.(loginAuth); ok {
		t.Fatal("PLAIN should be preferred when offered")
	}
	if _, err := pickAuth("CRAM-MD5", cfg); err == nil {
		t.Fatal("unsupported mechanisms accepted")
	}
}

func TestLoginAuthDialogue(t *testing.T) {
	a := loginAuth{user: "u@example.com", pass: "secret", host: "smtp.example.com"}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "smtp.example.com", TLS: false}); err == nil {
		t.Fatal("LOGIN allowed without TLS")
	}
	if mech, _, err := a.Start(&smtp.ServerInfo{Name: "smtp.example.com", TLS: true}); err != nil || mech != "LOGIN" {
		t.Fatalf("start: %q %v", mech, err)
	}
	if got, _ := a.Next([]byte("Username:"), true); string(got) != "u@example.com" {
		t.Fatalf("username reply %q", got)
	}
	if got, _ := a.Next([]byte("Password:"), true); string(got) != "secret" {
		t.Fatalf("password reply %q", got)
	}
	if _, err := a.Next([]byte("weird"), true); err == nil {
		t.Fatal("unexpected challenge accepted")
	}
}
