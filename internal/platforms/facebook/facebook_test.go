package facebook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/digkill/probot/internal/platforms"
)

func TestPublishFeedWithLinkAndPhoto(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("access_token") != "tok" {
			t.Errorf("missing token on %s", r.URL.Path)
		}
		got = append(got, r.URL.Path+"|"+r.Form.Get("message")+"|"+r.Form.Get("link")+"|"+r.Form.Get("url"))
		switch r.URL.Path {
		case "/123/feed":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "123_1"})
		case "/123/photos":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "p1", "post_id": "123_2"})
		}
	}))
	defer srv.Close()
	a := New()
	a.graph.Base = srv.URL
	creds := platforms.Credentials{AccessToken: "tok", ExternalRef: "123"}

	res, err := a.Publish(context.Background(), creds, platforms.NormalizedPost{Text: "hello", CTAURL: "https://prbo.ru"})
	if err != nil || res.ExternalID != "123_1" || res.URL != "https://www.facebook.com/123_1" {
		t.Fatalf("feed: %+v %v", res, err)
	}
	res, err = a.Publish(context.Background(), creds, platforms.NormalizedPost{Text: "pic", MediaURLs: []string{"https://x/a.jpg"}})
	if err != nil || res.ExternalID != "123_2" {
		t.Fatalf("photo: %+v %v", res, err)
	}
	if got[0] != "/123/feed|hello|https://prbo.ru|" || !strings.HasPrefix(got[1], "/123/photos||") || !strings.HasSuffix(got[1], "|https://x/a.jpg") {
		t.Fatalf("requests: %q", got)
	}
}

func TestGraphErrorDoesNotLeakToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid OAuth access token.","code":190}}`))
	}))
	defer srv.Close()
	a := New()
	a.graph.Base = srv.URL
	_, err := a.Connect(context.Background(), platforms.AuthInput{Token: "secret-token"})
	if err == nil || !strings.Contains(err.Error(), "Invalid OAuth") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("err=%v", err)
	}
}
