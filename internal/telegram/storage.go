package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/digkill/probot/internal/auth"
	"github.com/google/uuid"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/updates"
)

// encryptedStorage scopes all gotd storage to one account; an authenticated
// account/key binding prevents ciphertext being swapped between accounts.
type encryptedStorage struct {
	kv        KV
	id        uuid.UUID
	key       []byte
	mu        sync.Mutex
	failed    atomic.Bool
	onFailure func(error)
}

func (s *encryptedStorage) load(ctx context.Context, key string, v any) (bool, error) {
	b, err := s.kv.TelegramLoad(ctx, s.id, key)
	if err != nil {
		return false, safe(ErrStorage, err)
	}
	if len(b) == 0 {
		return false, nil
	}
	plain, err := auth.Open(s.key, b, []byte(s.id.String()+":"+key))
	if err != nil {
		return false, safe(ErrStorage, err)
	}
	if err := json.Unmarshal(plain, v); err != nil {
		return false, safe(ErrStorage, err)
	}
	return true, nil
}
func (s *encryptedStorage) save(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return safe(ErrStorage, err)
	}
	b, err = auth.Seal(s.key, b, []byte(s.id.String()+":"+key))
	if err != nil {
		return safe(ErrStorage, err)
	}
	err = s.kv.TelegramSave(ctx, s.id, key, b)
	if err != nil {
		s.failed.Store(true)
		if s.onFailure != nil {
			s.onFailure(safe(ErrStorage, err))
		}
	}
	return safe(ErrStorage, err)
}
func (s *encryptedStorage) LoadSession(ctx context.Context) ([]byte, error) {
	var b []byte
	ok, err := s.load(ctx, "session", &b)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, session.ErrNotFound
	}
	return b, nil
}
func (s *encryptedStorage) StoreSession(ctx context.Context, b []byte) error {
	return s.save(ctx, "session", b)
}

type Sessions struct {
	kv  KV
	key []byte
}

func NewSessions(kv KV, key string) (*Sessions, error) {
	if kv == nil || len(key) != 32 {
		return nil, ErrInvalidInput
	}
	return &Sessions{kv: kv, key: []byte(key)}, nil
}
func (s *Sessions) Load(ctx context.Context, id uuid.UUID) ([]byte, error) {
	return (&encryptedStorage{kv: s.kv, id: id, key: s.key}).LoadSession(ctx)
}
func (s *Sessions) Save(ctx context.Context, id uuid.UUID, b []byte) error {
	return (&encryptedStorage{kv: s.kv, id: id, key: s.key}).StoreSession(ctx, b)
}
func (s *Sessions) Delete(ctx context.Context, id uuid.UUID) error {
	return s.kv.TelegramDeleteState(ctx, id)
}

func (s *encryptedStorage) GetState(ctx context.Context, id int64) (updates.State, bool, error) {
	var v updates.State
	ok, err := s.load(ctx, fmt.Sprintf("updates/%d", id), &v)
	return v, ok, err
}
func (s *encryptedStorage) SetState(ctx context.Context, id int64, v updates.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed.Load() {
		return ErrStorage
	}
	return s.save(ctx, fmt.Sprintf("updates/%d", id), v)
}
func (s *encryptedStorage) change(ctx context.Context, id int64, fn func(*updates.State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed.Load() {
		return ErrStorage
	}
	v, ok, err := s.GetState(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("update state missing")
	}
	fn(&v)
	return s.save(ctx, fmt.Sprintf("updates/%d", id), v)
}
func (s *encryptedStorage) SetPts(ctx context.Context, id int64, v int) error {
	return s.change(ctx, id, func(s *updates.State) { s.Pts = v })
}
func (s *encryptedStorage) SetQts(ctx context.Context, id int64, v int) error {
	return s.change(ctx, id, func(s *updates.State) { s.Qts = v })
}
func (s *encryptedStorage) SetDate(ctx context.Context, id int64, v int) error {
	return s.change(ctx, id, func(s *updates.State) { s.Date = v })
}
func (s *encryptedStorage) SetSeq(ctx context.Context, id int64, v int) error {
	return s.change(ctx, id, func(s *updates.State) { s.Seq = v })
}
func (s *encryptedStorage) SetDateSeq(ctx context.Context, id int64, date, seq int) error {
	return s.change(ctx, id, func(s *updates.State) { s.Date = date; s.Seq = seq })
}
func (s *encryptedStorage) GetChannelPts(ctx context.Context, user, channel int64) (int, bool, error) {
	var v map[int64]int
	_, err := s.load(ctx, fmt.Sprintf("channels/%d", user), &v)
	p, ok := v[channel]
	return p, ok, err
}
func (s *encryptedStorage) SetChannelPts(ctx context.Context, user, channel int64, pts int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed.Load() {
		return ErrStorage
	}
	v := map[int64]int{}
	if _, err := s.load(ctx, fmt.Sprintf("channels/%d", user), &v); err != nil {
		return err
	}
	v[channel] = pts
	return s.save(ctx, fmt.Sprintf("channels/%d", user), v)
}
func (s *encryptedStorage) ForEachChannels(ctx context.Context, user int64, fn func(context.Context, int64, int) error) error {
	var v map[int64]int
	if _, err := s.load(ctx, fmt.Sprintf("channels/%d", user), &v); err != nil {
		return err
	}
	for id, pts := range v {
		if err := fn(ctx, id, pts); err != nil {
			return err
		}
	}
	return nil
}
func (s *encryptedStorage) SetChannelAccessHash(ctx context.Context, user, id, hash int64) error {
	return s.save(ctx, fmt.Sprintf("hash/channel/%d/%d", user, id), hash)
}
func (s *encryptedStorage) GetChannelAccessHash(ctx context.Context, user, id int64) (int64, bool, error) {
	var v int64
	ok, err := s.load(ctx, fmt.Sprintf("hash/channel/%d/%d", user, id), &v)
	return v, ok, err
}
func (s *encryptedStorage) SetUserAccessHash(ctx context.Context, user, id, hash int64) error {
	return s.save(ctx, fmt.Sprintf("hash/user/%d/%d", user, id), hash)
}
func (s *encryptedStorage) GetUserAccessHash(ctx context.Context, user, id int64) (int64, bool, error) {
	var v int64
	ok, err := s.load(ctx, fmt.Sprintf("hash/user/%d/%d", user, id), &v)
	return v, ok, err
}
func (s *encryptedStorage) Save(ctx context.Context, k peers.Key, v peers.Value) error {
	return s.save(ctx, fmt.Sprintf("peer/%s/%d", k.Prefix, k.ID), v)
}
func (s *encryptedStorage) Find(ctx context.Context, k peers.Key) (peers.Value, bool, error) {
	var v peers.Value
	ok, err := s.load(ctx, fmt.Sprintf("peer/%s/%d", k.Prefix, k.ID), &v)
	return v, ok, err
}
func (s *encryptedStorage) SavePhone(ctx context.Context, phone string, k peers.Key) error {
	return s.save(ctx, "phone/"+phone, k)
}
func (s *encryptedStorage) FindPhone(ctx context.Context, phone string) (peers.Key, peers.Value, bool, error) {
	var k peers.Key
	ok, err := s.load(ctx, "phone/"+phone, &k)
	if err != nil || !ok {
		return k, peers.Value{}, ok, err
	}
	v, ok, err := s.Find(ctx, k)
	return k, v, ok, err
}
func (s *encryptedStorage) GetContactsHash(ctx context.Context) (int64, error) {
	var v int64
	_, err := s.load(ctx, "contacts_hash", &v)
	return v, err
}
func (s *encryptedStorage) SaveContactsHash(ctx context.Context, v int64) error {
	return s.save(ctx, "contacts_hash", v)
}

var _ updates.StateStorage = (*encryptedStorage)(nil)
var _ peers.Storage = (*encryptedStorage)(nil)
var _ session.Storage = (*encryptedStorage)(nil)
