package linkedin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digkill/probot/internal/platforms"
)

func TestConnectNormalizesAuthor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sub":"abc123","name":"Alex"}`))
	}))
	defer srv.Close()
	a := New()
	a.base = srv.URL
	for in, want := range map[string]string{
		"":                       "urn:li:person:abc123",
		"12345":                  "urn:li:organization:12345",
		"urn:li:organization:77": "urn:li:organization:77",
	} {
		c, err := a.Connect(context.Background(), platforms.AuthInput{Token: "t", ExternalRef: in})
		if err != nil || c.ExternalRef != want {
			t.Fatalf("%q -> %q, %v", in, c.ExternalRef, err)
		}
	}
	if _, err := a.Connect(context.Background(), platforms.AuthInput{Token: "t", ExternalRef: "some-company"}); err == nil {
		t.Fatal("bad external_ref accepted")
	}
}

func TestPublishSendsEscapedCommentaryAndArticle(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/posts" || r.Header.Get("LinkedIn-Version") != apiVersion || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("bad request %s %v", r.URL.Path, r.Header)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("x-restli-id", "urn:li:share:999")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	a := New()
	a.base = srv.URL
	res, err := a.Publish(context.Background(), platforms.Credentials{AccessToken: "tok", ExternalRef: "urn:li:organization:1"},
		platforms.NormalizedPost{Text: "Launch (beta) #ai @team\nmore", CTAURL: "https://prbo.ru"})
	if err != nil || res.ExternalID != "urn:li:share:999" || res.URL != "https://www.linkedin.com/feed/update/urn:li:share:999/" {
		t.Fatalf("%+v %v", res, err)
	}
	if body["commentary"] != `Launch \(beta\) \#ai \@team`+"\nmore" || body["author"] != "urn:li:organization:1" {
		t.Fatalf("body=%v", body)
	}
	article := body["content"].(map[string]any)["article"].(map[string]any)
	if article["source"] != "https://prbo.ru" || article["title"] != "Launch (beta) #ai @team" {
		t.Fatalf("article=%v", article)
	}
}
