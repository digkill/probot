package threads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/digkill/probot/internal/platforms"
)

func TestPublishTextPostFitsLimitAndKeepsLink(t *testing.T) {
	var text, mediaType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/7/threads":
			text, mediaType = r.Form.Get("text"), r.Form.Get("media_type")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "c1"})
		case "/7/threads_publish":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "t1"})
		case "/t1":
			_ = json.NewEncoder(w).Encode(map[string]string{"permalink": "https://threads.net/@me/post/t1"})
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	a := New()
	a.graph.Base = srv.URL
	res, err := a.Publish(context.Background(), platforms.Credentials{AccessToken: "tok", ExternalRef: "7"},
		platforms.NormalizedPost{Text: strings.Repeat("й", 600), CTAURL: "https://prbo.ru"})
	if err != nil || res.ExternalID != "t1" {
		t.Fatalf("%+v %v", res, err)
	}
	if mediaType != "TEXT" || len([]rune(text)) > textLimit || !strings.HasSuffix(text, "https://prbo.ru") {
		t.Fatalf("type=%s len=%d", mediaType, len([]rune(text)))
	}
}

func TestStatsParsesInsights(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"name":"views","values":[{"value":120}]},{"name":"likes","values":[{"value":9}]},{"name":"replies","total_value":{"value":3}},{"name":"reposts","values":[{"value":2}]}]}`))
	}))
	defer srv.Close()
	a := New()
	a.graph.Base = srv.URL
	st, err := a.FetchStats(context.Background(), platforms.Credentials{AccessToken: "tok"}, "t1")
	if err != nil || st.Reach != 120 || st.Likes != 9 || st.Comments != 3 || st.Shares != 2 {
		t.Fatalf("%+v %v", st, err)
	}
}
