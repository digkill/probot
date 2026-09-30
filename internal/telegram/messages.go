package telegram

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
	"github.com/gotd/td/constant"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

func peerID(p tg.PeerClass) int64 {
	switch p := p.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return -p.ChatID
	case *tg.PeerChannel:
		return -1000000000000 - p.ChannelID
	}
	return 0
}
func inputID(p tg.InputPeerClass, self int64) int64 {
	switch p := p.(type) {
	case *tg.InputPeerSelf:
		return self
	case *tg.InputPeerUser:
		return p.UserID
	case *tg.InputPeerChat:
		return -p.ChatID
	case *tg.InputPeerChannel:
		return -1000000000000 - p.ChannelID
	}
	return 0
}
func mapMessage(id uuid.UUID, raw tg.MessageClass) (domain.TelegramMessage, bool) {
	if m, ok := raw.(*tg.MessageService); ok {
		return domain.TelegramMessage{ID: m.ID, AccountID: id, ChatID: peerID(m.PeerID), SenderID: peerID(m.FromID), Date: time.Unix(int64(m.Date), 0).UTC(), Outgoing: m.Out, Service: true}, true
	}
	m, ok := raw.(*tg.Message)
	if !ok {
		return domain.TelegramMessage{}, false
	}
	sender := peerID(m.FromID)
	chat := peerID(m.PeerID)
	if sender == 0 && chat > 0 && !m.Out {
		sender = chat
	}
	return domain.TelegramMessage{ID: m.ID, AccountID: id, ChatID: chat, SenderID: sender, Text: m.Message, Date: time.Unix(int64(m.Date), 0).UTC(), Outgoing: m.Out, HasMedia: m.Media != nil}, true
}
func peerDTO(p peers.Peer) domain.TelegramPeer {
	id := int64(p.TDLibPeerID())
	kind := "user"
	if id <= -1000000000000 {
		kind = "channel"
	} else if id < 0 {
		kind = "chat"
	}
	name, _ := p.Username()
	return domain.TelegramPeer{ID: id, Type: kind, Title: p.VisibleName(), Username: name}
}

type peerResolver struct{ c *gotdClient }
type peerAlias struct {
	ID      int64
	Expires time.Time
}

func (r peerResolver) ResolveDomain(ctx context.Context, name string) (tg.InputPeerClass, error) {
	name = strings.ToLower(strings.TrimPrefix(name, "@"))
	var cached peerAlias
	ok, err := r.c.storage.load(ctx, "username/"+name, &cached)
	if err != nil {
		return nil, err
	}
	if ok && time.Now().Before(cached.Expires) {
		p, err := r.c.peers.ResolveTDLibID(ctx, constant.TDLibPeerID(cached.ID))
		if err == nil {
			return p.InputPeer(), nil
		}
	}
	p, err := r.c.peers.ResolveDomain(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := r.c.storage.save(ctx, "username/"+name, peerAlias{ID: int64(p.TDLibPeerID()), Expires: time.Now().Add(5 * time.Minute)}); err != nil {
		return nil, err
	}
	return p.InputPeer(), nil
}
func (r peerResolver) ResolvePhone(ctx context.Context, phone string) (tg.InputPeerClass, error) {
	p, err := r.c.peers.ResolvePhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	return p.InputPeer(), nil
}
func (c *gotdClient) resolve(ctx context.Context, raw string) (tg.InputPeerClass, error) {
	raw = strings.TrimSpace(raw)
	if raw == "me" {
		return &tg.InputPeerSelf{}, nil
	}
	if len(raw) == 0 || len(raw) > 64 {
		return nil, ErrInvalidInput
	}
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if id == 0 {
			return nil, ErrInvalidInput
		}
		p, err := c.peers.ResolveTDLibID(ctx, constant.TDLibPeerID(id))
		if err != nil {
			return nil, safe(ErrPeerNotFound, err)
		}
		return p.InputPeer(), nil
	}
	p, err := c.resolver.ResolveDomain(ctx, strings.TrimPrefix(raw, "@"))
	return p, rpcError(err)
}
func (c *gotdClient) Resolve(ctx context.Context, raw string) (domain.TelegramPeer, error) {
	p, err := c.resolve(ctx, raw)
	if err != nil {
		return domain.TelegramPeer{}, err
	}
	v, err := c.peers.FromInputPeer(ctx, p)
	if err != nil {
		return domain.TelegramPeer{}, rpcError(err)
	}
	return peerDTO(v), nil
}

func (c *gotdClient) SendMessage(ctx context.Context, req domain.TelegramSendRequest) (*domain.TelegramMessage, error) {
	if strings.TrimSpace(req.Text) == "" || utf8.RuneCountInString(req.Text) > 4096 {
		return nil, ErrInvalidInput
	}
	p, err := c.resolve(ctx, req.Peer)
	if err != nil {
		return nil, err
	}
	result, err := c.sender.To(p).Text(ctx, req.Text)
	if err != nil {
		return nil, rpcError(err)
	}
	var raw []tg.UpdateClass
	switch u := result.(type) {
	case *tg.Updates:
		raw = u.Updates
	case *tg.UpdatesCombined:
		raw = u.Updates
	case *tg.UpdateShort:
		raw = []tg.UpdateClass{u.Update}
	case *tg.UpdateShortSentMessage:
		m := domain.TelegramMessage{ID: u.ID, AccountID: c.account.ID, ChatID: inputID(p, c.selfID.Load()), SenderID: c.selfID.Load(), Text: req.Text, Date: time.Unix(int64(u.Date), 0).UTC(), Outgoing: true, HasMedia: u.Media != nil}
		err := c.events.PublishTelegramEvent(ctx, domain.TelegramEvent{AccountID: c.account.ID, Type: domain.TelegramMessageSent, Message: m})
		return &m, safe(ErrStorage, err)
	}
	for _, u := range raw {
		var msg tg.MessageClass
		switch u := u.(type) {
		case *tg.UpdateNewMessage:
			msg = u.Message
		case *tg.UpdateNewChannelMessage:
			msg = u.Message
		}
		if m, ok := mapMessage(c.account.ID, msg); ok && m.Outgoing && m.ChatID == inputID(p, c.selfID.Load()) {
			if m.SenderID == 0 {
				m.SenderID = c.selfID.Load()
			}
			err := c.events.PublishTelegramEvent(ctx, domain.TelegramEvent{AccountID: c.account.ID, Type: domain.TelegramMessageSent, Message: m})
			return &m, safe(ErrStorage, err)
		}
	}
	return nil, ErrUnavailable
}
func (c *gotdClient) GetHistory(ctx context.Context, raw string, limit, offset int) ([]domain.TelegramMessage, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalidInput
	}
	p, err := c.resolve(ctx, raw)
	if err != nil {
		return nil, err
	}
	result, err := c.client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: p, Limit: limit, OffsetID: offset})
	if err != nil {
		return nil, rpcError(err)
	}
	modified, ok := result.AsModified()
	if !ok {
		return []domain.TelegramMessage{}, nil
	}
	if err := c.peers.Apply(ctx, modified.GetUsers(), modified.GetChats()); err != nil {
		return nil, rpcError(err)
	}
	out := []domain.TelegramMessage{}
	for _, raw := range modified.GetMessages() {
		if m, ok := mapMessage(c.account.ID, raw); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

type dialogCursor struct {
	ID   int   `json:"id"`
	Date int   `json:"date"`
	Peer int64 `json:"peer"`
}

func (c *gotdClient) GetDialogs(ctx context.Context, limit int, cursor string) (domain.TelegramDialogs, error) {
	out := domain.TelegramDialogs{Items: []domain.TelegramDialog{}}
	if limit < 1 || limit > 100 || len(cursor) > 256 {
		return out, ErrInvalidInput
	}
	req := &tg.MessagesGetDialogsRequest{Limit: limit, OffsetPeer: &tg.InputPeerEmpty{}}
	if cursor != "" {
		var cur dialogCursor
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(b, &cur) != nil || cur.ID <= 0 || cur.Date <= 0 {
			return out, ErrInvalidInput
		}
		p, err := c.resolve(ctx, strconv.FormatInt(cur.Peer, 10))
		if err != nil {
			return out, err
		}
		req.OffsetID = cur.ID
		req.OffsetDate = cur.Date
		req.OffsetPeer = p
		req.ExcludePinned = true
	}
	result, err := c.client.API().MessagesGetDialogs(ctx, req)
	if err != nil {
		return out, rpcError(err)
	}
	var ds []tg.DialogClass
	var ms []tg.MessageClass
	var us []tg.UserClass
	var cs []tg.ChatClass
	switch r := result.(type) {
	case *tg.MessagesDialogs:
		ds = r.Dialogs
		ms = r.Messages
		us = r.Users
		cs = r.Chats
	case *tg.MessagesDialogsSlice:
		ds = r.Dialogs
		ms = r.Messages
		us = r.Users
		cs = r.Chats
	default:
		return out, nil
	}
	if err := c.peers.Apply(ctx, us, cs); err != nil {
		return out, rpcError(err)
	}
	var last *tg.Dialog
	for _, raw := range ds {
		d, ok := raw.(*tg.Dialog)
		if !ok {
			continue
		}
		p, err := c.peers.ResolvePeer(ctx, d.Peer)
		if err != nil {
			return out, rpcError(err)
		}
		out.Items = append(out.Items, domain.TelegramDialog{TelegramPeer: peerDTO(p), UnreadCount: d.UnreadCount})
		last = d
	}
	if len(ds) >= limit && last != nil {
		for _, raw := range ms {
			m, ok := raw.AsNotEmpty()
			if ok && m.GetID() == last.TopMessage && peerID(m.GetPeerID()) == peerID(last.Peer) {
				b, _ := json.Marshal(dialogCursor{ID: m.GetID(), Date: m.GetDate(), Peer: peerID(last.Peer)})
				out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
				break
			}
		}
	}
	return out, nil
}
func (c *gotdClient) EditMessage(ctx context.Context, raw string, id int, text string) error {
	if id <= 0 || strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 4096 {
		return ErrInvalidInput
	}
	p, err := c.resolve(ctx, raw)
	if err != nil {
		return err
	}
	_, err = c.sender.To(p).Edit(id).Text(ctx, text)
	return rpcError(err)
}
func (c *gotdClient) DeleteMessage(ctx context.Context, raw string, id int, revoke bool) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	p, err := c.resolve(ctx, raw)
	if err != nil {
		return err
	}
	if ch, ok := p.(*tg.InputPeerChannel); ok {
		_, err = c.client.API().ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash}, ID: []int{id}})
	} else {
		// IDs are global for non-channel messages: verify the requested peer first.
		result, e := c.client.API().MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: id}})
		if e != nil {
			return rpcError(e)
		}
		v, ok := result.AsModified()
		found := false
		if ok {
			for _, raw := range v.GetMessages() {
				m, ok := raw.(*tg.Message)
				if ok && peerID(m.PeerID) == inputID(p, c.selfID.Load()) {
					found = true
				}
			}
		}
		if !found {
			return ErrInvalidInput
		}
		_, err = c.client.API().MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{ID: []int{id}, Revoke: revoke})
	}
	return rpcError(err)
}
func (c *gotdClient) ForwardMessage(ctx context.Context, from, to string, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	src, err := c.resolve(ctx, from)
	if err != nil {
		return err
	}
	dst, err := c.resolve(ctx, to)
	if err != nil {
		return err
	}
	_, err = c.sender.To(dst).ForwardIDs(src, id).Send(ctx)
	return rpcError(err)
}
