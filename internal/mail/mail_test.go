package mail

import (
	"context"
	"encoding/base64"
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
