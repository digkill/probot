package telegram

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

type fakeEvents struct {
	mu     sync.Mutex
	events []domain.TelegramEvent
	err    error
	notify chan struct{}
}

func (f *fakeEvents) PublishTelegramEvent(ctx context.Context, e domain.TelegramEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, e)
	if f.notify != nil {
		select {
		case f.notify <- struct{}{}:
		default:
		}
	}
	return nil
}
func TestMessageMapping(t *testing.T) {
	id := uuid.New()
	for _, tt := range []struct {
		peer tg.PeerClass
		want int64
	}{
		{&tg.PeerUser{UserID: 42}, 42}, {&tg.PeerChat{ChatID: 42}, -42}, {&tg.PeerChannel{ChannelID: 42}, -1000000000042},
	} {
		m, ok := mapMessage(id, &tg.Message{ID: 7, PeerID: tt.peer, FromID: &tg.PeerUser{UserID: 8}, Message: "text", Date: 100, Out: true})
		if !ok || m.ChatID != tt.want || m.SenderID != 8 || m.ID != 7 || m.AccountID != id || !m.Outgoing || m.Date.Unix() != 100 {
			t.Fatalf("%+v", m)
		}
	}
	if m, ok := mapMessage(id, &tg.MessageService{ID: 9, PeerID: &tg.PeerChat{ChatID: 42}, Date: 100}); !ok || !m.Service || m.ID != 9 {
		t.Fatal("service message lost history cursor")
	}
	if _, ok := mapMessage(id, &tg.MessageEmpty{}); ok {
		t.Fatal("empty message mapped")
	}
}
func TestUpdateEventFailureFreezesRecoveryState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newMemoryRepo()
	id := uuid.New()
	s := storageForTest(r, id)
	if err := s.SetState(ctx, 1, updates.State{Pts: 10}); err != nil {
		t.Fatal(err)
	}
	sink := &fakeEvents{err: errors.New("database unavailable")}
	c := &gotdClient{account: domain.TelegramAccount{ID: id}, storage: s, events: sink, cancel: cancel, fatal: make(chan error, 1)}
	err := c.receive(ctx, &tg.Message{ID: 2, PeerID: &tg.PeerUser{UserID: 3}, Message: "private"})
	if !errors.Is(err, ErrStorage) {
		t.Fatal(err)
	}
	if err := s.SetPts(context.Background(), 1, 11); !errors.Is(err, ErrStorage) {
		t.Fatal("pts advanced after event failure")
	}
	reloaded := storageForTest(r, id)
	state, _, err := reloaded.GetState(context.Background(), 1)
	if err != nil || state.Pts != 10 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if ctx.Err() == nil {
		t.Fatal("runtime not cancelled")
	}
}

type differenceAPI struct {
	mu        sync.Mutex
	requested []int
}

func (f *differenceAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	return &tg.UpdatesState{Pts: 11, Date: 100, Seq: 1}, nil
}
func (f *differenceAPI) UpdatesGetDifference(ctx context.Context, r *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	f.mu.Lock()
	f.requested = append(f.requested, r.Pts)
	f.mu.Unlock()
	return &tg.UpdatesDifference{NewMessages: []tg.MessageClass{&tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, FromID: &tg.PeerUser{UserID: 42}, Message: "missed", Date: 100}}, Users: []tg.UserClass{&tg.User{ID: 42, AccessHash: 123, FirstName: "Test"}}, State: tg.UpdatesState{Pts: 11, Date: 100, Seq: 1}}, nil
}
func (f *differenceAPI) UpdatesGetChannelDifference(context.Context, *tg.UpdatesGetChannelDifferenceRequest) (tg.UpdatesChannelDifferenceClass, error) {
	return &tg.UpdatesChannelDifferenceEmpty{Pts: 1, Final: true}, nil
}
func TestRecoveryDispatchesOfflineMessageFromPersistentPTS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newMemoryRepo()
	id := uuid.New()
	stored := storageForTest(r, id)
	if err := stored.SetState(ctx, 1, updates.State{Pts: 10, Date: 99, Seq: 1}); err != nil {
		t.Fatal(err)
	}
	sink := &fakeEvents{notify: make(chan struct{}, 1)}
	factory, err := NewFactory(Config{AppID: 1, AppHash: "test-only", EncryptionKey: testKey, RPCRate: 1, AuthTTL: time.Minute}, r, sink, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	client, err := factory(domain.TelegramAccount{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	c := client.(*gotdClient)
	c.cancel = cancel
	api := &differenceAPI{}
	done := make(chan error, 1)
	go func() { done <- c.gaps.Run(ctx, api, 1, updates.AuthOptions{}) }()
	select {
	case <-sink.notify:
	case err := <-done:
		t.Fatalf("recovery stopped: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("missed update not delivered")
	}
	// Wait until gotd has checkpointed the delivered event before simulating restart.
	deadline := time.Now().Add(time.Second)
	for {
		v, _, err := stored.GetState(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if v.Pts == 11 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("checkpoint not saved")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recovery leaked goroutine")
	}
	api.mu.Lock()
	first := api.requested[0]
	api.mu.Unlock()
	if first != 10 {
		t.Fatalf("recovered from pts=%d", first)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 1 || sink.events[0].Type != domain.TelegramMessageReceived || sink.events[0].Message.Text != "missed" {
		t.Fatalf("events=%+v", sink.events)
	}
}

func TestDispatcherNormalizesIncomingChannelAndOutgoingEvents(t *testing.T) {
	r := newMemoryRepo()
	sink := &fakeEvents{}
	id := uuid.New()
	factory, err := NewFactory(Config{AppID: 1, AppHash: "test", EncryptionKey: testKey, RPCRate: 1, AuthTTL: time.Minute}, r, sink, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	client, err := factory(domain.TelegramAccount{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	c := client.(*gotdClient)
	err = c.gaps.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 1, PeerID: &tg.PeerChannel{ChannelID: 5}, FromID: &tg.PeerUser{UserID: 7}, Message: "incoming", Date: 100}},
		&tg.UpdateNewMessage{Message: &tg.Message{ID: 2, PeerID: &tg.PeerUser{UserID: 7}, Out: true, Message: "outgoing", Date: 101}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 2 || sink.events[0].Type != domain.TelegramMessageReceived || sink.events[0].Message.ChatID != -1000000000005 || sink.events[1].Type != domain.TelegramMessageSent {
		t.Fatalf("events=%+v", sink.events)
	}
}
