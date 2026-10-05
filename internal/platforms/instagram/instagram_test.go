package instagram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/digkill/probot/internal/platforms"
)

func TestPublishWaitsForContainerThenPublishes(t *testing.T) {
	polls := 0
	var caption string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/42/media":
			caption = r.Form.Get("caption")
			if r.Form.Get("image_url") != "https://x/a.jpg" {
				t.Errorf("image_url=%q", r.Form.Get("image_url"))
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "c1"})
		case r.URL.Path == "/c1":
			polls++
			status := "IN_PROGRESS"
			if polls > 1 {
				status = "FINISHED"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status_code": status})
		case r.URL.Path == "/42/media_publish":
			if r.Form.Get("creation_id") != "c1" {
				t.Errorf("creation_id=%q", r.Form.Get("creation_id"))
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "m1"})
		case r.URL.Path == "/m1":
			_ = json.NewEncoder(w).Encode(map[string]string{"permalink": "https://instagram.com/p/abc"})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	a := New()
	a.fb.Base = srv.URL
	a.pollStep = 0
	res, err := a.Publish(context.Background(), platforms.Credentials{AccessToken: "EAAtok", ExternalRef: "42"},
		platforms.NormalizedPost{Text: "launch", CTAURL: "https://prbo.ru", MediaURLs: []string{"https://x/a.jpg"}})
	if err != nil || res.ExternalID != "m1" || res.URL != "https://instagram.com/p/abc" {
		t.Fatalf("%+v %v", res, err)
	}
	if polls < 2 || caption != "launch\n\nhttps://prbo.ru" {
		t.Fatalf("polls=%d caption=%q", polls, caption)
	}
}

func TestPublishRequiresMedia(t *testing.T) {
	_, err := New().Publish(context.Background(), platforms.Credentials{AccessToken: "t", ExternalRef: "42"}, platforms.NormalizedPost{Text: "x"})
	if err == nil || !strings.Contains(err.Error(), "image or video") {
		t.Fatalf("err=%v", err)
	}
}

func TestInstagramLoginTokensUseInstagramHost(t *testing.T) {
	a := New()
	if a.client("IGQVJtok") != a.ig || a.client("EAAtok") != a.fb {
		t.Fatal("wrong graph host selection")
	}
}
