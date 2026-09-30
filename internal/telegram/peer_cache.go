package telegram

import (
	"context"
	"fmt"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

type peerCache struct{ storage *encryptedStorage }

func (c peerCache) SaveUsers(ctx context.Context, values ...*tg.User) error {
	for _, v := range values {
		var b bin.Buffer
		if err := v.Encode(&b); err != nil {
			return err
		}
		if err := c.storage.save(ctx, fmt.Sprintf("entity/User/%d", v.ID), b.Buf); err != nil {
			return err
		}
	}
	return nil
}
func (c peerCache) FindUser(ctx context.Context, id int64) (*tg.User, bool, error) {
	var b []byte
	ok, err := c.storage.load(ctx, fmt.Sprintf("entity/User/%d", id), &b)
	if err != nil || !ok {
		return nil, ok, err
	}
	v := new(tg.User)
	if err := v.Decode(&bin.Buffer{Buf: b}); err != nil {
		return nil, false, safe(ErrStorage, err)
	}
	return v, true, nil
}
func (c peerCache) SaveUserFulls(ctx context.Context, values ...*tg.UserFull) error {
	for _, v := range values {
		var b bin.Buffer
		if err := v.Encode(&b); err != nil {
			return err
		}
		if err := c.storage.save(ctx, fmt.Sprintf("entity/UserFull/%d", v.ID), b.Buf); err != nil {
			return err
		}
	}
	return nil
}
func (c peerCache) FindUserFull(ctx context.Context, id int64) (*tg.UserFull, bool, error) {
	var b []byte
	ok, err := c.storage.load(ctx, fmt.Sprintf("entity/UserFull/%d", id), &b)
	if err != nil || !ok {
		return nil, ok, err
	}
	v := new(tg.UserFull)
	if err := v.Decode(&bin.Buffer{Buf: b}); err != nil {
		return nil, false, safe(ErrStorage, err)
	}
	return v, true, nil
}
func (c peerCache) SaveChats(ctx context.Context, values ...*tg.Chat) error {
	for _, v := range values {
		var b bin.Buffer
		if err := v.Encode(&b); err != nil {
			return err
		}
		if err := c.storage.save(ctx, fmt.Sprintf("entity/Chat/%d", v.ID), b.Buf); err != nil {
			return err
		}
	}
	return nil
}
func (c peerCache) FindChat(ctx context.Context, id int64) (*tg.Chat, bool, error) {
	var b []byte
	ok, err := c.storage.load(ctx, fmt.Sprintf("entity/Chat/%d", id), &b)
	if err != nil || !ok {
		return nil, ok, err
	}
	v := new(tg.Chat)
	if err := v.Decode(&bin.Buffer{Buf: b}); err != nil {
		return nil, false, safe(ErrStorage, err)
	}
	return v, true, nil
}
func (c peerCache) SaveChatFulls(ctx context.Context, values ...*tg.ChatFull) error {
	for _, v := range values {
		var b bin.Buffer
		if err := v.Encode(&b); err != nil {
			return err
		}
		if err := c.storage.save(ctx, fmt.Sprintf("entity/ChatFull/%d", v.ID), b.Buf); err != nil {
			return err
		}
	}
	return nil
}
func (c peerCache) FindChatFull(ctx context.Context, id int64) (*tg.ChatFull, bool, error) {
	var b []byte
	ok, err := c.storage.load(ctx, fmt.Sprintf("entity/ChatFull/%d", id), &b)
	if err != nil || !ok {
		return nil, ok, err
	}
	v := new(tg.ChatFull)
	if err := v.Decode(&bin.Buffer{Buf: b}); err != nil {
		return nil, false, safe(ErrStorage, err)
	}
	return v, true, nil
}
func (c peerCache) SaveChannels(ctx context.Context, values ...*tg.Channel) error {
	for _, v := range values {
		var b bin.Buffer
		if err := v.Encode(&b); err != nil {
			return err
		}
		if err := c.storage.save(ctx, fmt.Sprintf("entity/Channel/%d", v.ID), b.Buf); err != nil {
			return err
		}
	}
	return nil
}
func (c peerCache) FindChannel(ctx context.Context, id int64) (*tg.Channel, bool, error) {
	var b []byte
	ok, err := c.storage.load(ctx, fmt.Sprintf("entity/Channel/%d", id), &b)
	if err != nil || !ok {
		return nil, ok, err
	}
	v := new(tg.Channel)
	if err := v.Decode(&bin.Buffer{Buf: b}); err != nil {
		return nil, false, safe(ErrStorage, err)
	}
	return v, true, nil
}
func (c peerCache) SaveChannelFulls(ctx context.Context, values ...*tg.ChannelFull) error {
	for _, v := range values {
		var b bin.Buffer
		if err := v.Encode(&b); err != nil {
			return err
		}
		if err := c.storage.save(ctx, fmt.Sprintf("entity/ChannelFull/%d", v.ID), b.Buf); err != nil {
			return err
		}
	}
	return nil
}
func (c peerCache) FindChannelFull(ctx context.Context, id int64) (*tg.ChannelFull, bool, error) {
	var b []byte
	ok, err := c.storage.load(ctx, fmt.Sprintf("entity/ChannelFull/%d", id), &b)
	if err != nil || !ok {
		return nil, ok, err
	}
	v := new(tg.ChannelFull)
	if err := v.Decode(&bin.Buffer{Buf: b}); err != nil {
		return nil, false, safe(ErrStorage, err)
	}
	return v, true, nil
}

var _ peers.Cache = peerCache{}
