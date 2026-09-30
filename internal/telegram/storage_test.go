package telegram

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

const testKey = "test-only-key-012345678901234567"

func storageForTest(r *memoryRepo, id uuid.UUID) *encryptedStorage {
	return &encryptedStorage{kv: r, id: id, key: []byte(testKey)}
}
func TestSessionEncryptedIsolatedAndRestarted(t *testing.T) {
	r := newMemoryRepo()
	id, other := uuid.New(), uuid.New()
	s, err := NewSessions(r, testKey)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.Load(ctx, id); !errors.Is(err, session.ErrNotFound) {
		t.Fatal(err)
	}
	secret := []byte(`{"auth_key":"not-a-real-key"}`)
	if err := s.Save(ctx, id, secret); err != nil {
		t.Fatal(err)
	}
	for _, v := range r.values {
		if bytes.Contains(v, secret) {
			t.Fatal("plaintext session stored")
		}
	}
	restarted, _ := NewSessions(r, testKey)
	v, err := restarted.Load(ctx, id)
	if err != nil || !bytes.Equal(v, secret) {
		t.Fatalf("roundtrip err=%v", err)
	}
	r.values[other.String()+"session"] = append([]byte(nil), r.values[id.String()+"session"]...)
	if _, err := restarted.Load(ctx, other); !errors.Is(err, ErrStorage) {
		t.Fatalf("swapped ciphertext accepted: %v", err)
	}
	if err := s.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, id); !errors.Is(err, session.ErrNotFound) {
		t.Fatal(err)
	}
}
func TestUpdateStateAndPeerStorageSurviveRestart(t *testing.T) {
	r := newMemoryRepo()
	id := uuid.New()
	s := storageForTest(r, id)
	ctx := context.Background()
	if err := s.SetPts(ctx, 42, 1); err == nil {
		t.Fatal("missing state silently created")
	}
	if err := s.SetState(ctx, 42, updates.State{Pts: 1, Qts: 2, Date: 3, Seq: 4}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 1; i <= 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.SetChannelPts(ctx, 42, int64(i), i); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if err := s.SetDateSeq(ctx, 42, 7, 8); err != nil {
		t.Fatal(err)
	}
	key := peers.Key{Prefix: "users_", ID: 55}
	if err := s.Save(ctx, key, peers.Value{AccessHash: 987}); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePhone(ctx, "15551234567", key); err != nil {
		t.Fatal(err)
	}
	cache := peerCache{s}
	user := &tg.User{ID: 55, FirstName: "Test", Username: "example", AccessHash: 987, Photo: &tg.UserProfilePhotoEmpty{}}
	if err := cache.SaveUsers(ctx, user); err != nil {
		t.Fatal(err)
	}
	next := storageForTest(r, id)
	state, ok, err := next.GetState(ctx, 42)
	if err != nil || !ok || state.Pts != 1 || state.Qts != 2 || state.Date != 7 || state.Seq != 8 {
		t.Fatalf("state=%+v %v", state, err)
	}
	count := 0
	if err := next.ForEachChannels(ctx, 42, func(_ context.Context, id int64, pts int) error {
		count++
		if int(id) != pts {
			t.Fatal("channel pts mismatch")
		}
		return nil
	}); err != nil || count != 20 {
		t.Fatalf("channels=%d err=%v", count, err)
	}
	_, v, ok, err := next.FindPhone(ctx, "15551234567")
	if err != nil || !ok || v.AccessHash != 987 {
		t.Fatal("peer lost")
	}
	restored, ok, err := (peerCache{next}).FindUser(ctx, 55)
	if err != nil || !ok || restored.Username != "example" {
		t.Fatalf("entity lost: %v", err)
	}
	other := storageForTest(r, uuid.New())
	if _, ok, _ := other.GetState(ctx, 42); ok {
		t.Fatal("cross-account state")
	}
}
func TestStorageCancellation(t *testing.T) {
	s := storageForTest(newMemoryRepo(), uuid.New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.StoreSession(ctx, []byte("secret")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
