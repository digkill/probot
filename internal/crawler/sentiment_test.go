package crawler

import (
	"strings"
	"testing"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
)

func TestClassifyNegativeHigh(t *testing.T) {
	sent, sev := Classify("This app is a scam", "They stole money, refund now, fraud")
	if sent != "negative" || sev != "high" {
		t.Fatalf("got %s/%s", sent, sev)
	}
}

func TestClassifyReviewWordIsNotNegative(t *testing.T) {
	sent, _ := Classify("Новый отзыв о сервисе", "Пользователь оставил отзыв на сайте")
	if sent == "negative" {
		t.Fatalf("generic review should not be negative, got %s", sent)
	}
}

func TestClassifyRussianScam(t *testing.T) {
	sent, _ := Classify("Это развод", "Мошенники, кинули, не рекомендую")
	if sent != "negative" {
		t.Fatalf("got %s", sent)
	}
}

func TestClassifyPositive(t *testing.T) {
	sent, _ := Classify("Love this", "Great product, recommend")
	if sent != "positive" {
		t.Fatalf("got %s", sent)
	}
}

func TestKeepForWatch(t *testing.T) {
	if KeepForWatch("reviews", "positive") {
		t.Fatal("should drop positive review-watch hits")
	}
	if !KeepForWatch("reviews", "negative") {
		t.Fatal("should keep negatives")
	}
	if !KeepForWatch("mentions", "positive") {
		t.Fatal("generic crawlers keep everything")
	}
}

func TestBrandWatchQueryIncludesHost(t *testing.T) {
	q := BrandWatchQuery("Acme", "acme", "https://www.acme.dev")
	if !strings.Contains(q, "Acme") || !strings.Contains(q, "acme.dev") {
		t.Fatalf("query=%s", q)
	}
}

func TestDraftObjectionRussian(t *testing.T) {
	brand := &domain.Brand{ID: uuid.New(), Name: "Акаме", CanonicalURL: "https://acme.dev"}
	text := DraftObjection(domain.Mention{Title: "Это развод", Snippet: "кинули"}, brand)
	if !LooksRussian(text) {
		t.Fatalf("expected Russian draft, got %s", text)
	}
}

func TestStripHTML(t *testing.T) {
	got := StripHTML("<p>Hello&nbsp;<b>world</b></p>")
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "world") || strings.Contains(got, "<") {
		t.Fatalf("got %q", got)
	}
}
