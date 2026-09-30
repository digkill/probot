package telegram

import (
	"context"
	"io"
	"path/filepath"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// File transfers use bounded chunks; readers/writers must honor cancellation
// themselves when their Read/Write can block (HTTP bodies do).
func (c *gotdClient) SendFile(ctx context.Context, peer, name string, size int64, src io.Reader, photo bool) error {
	if src == nil || size <= 0 || size > 2<<30 || name == "" || len(name) > 255 {
		return ErrInvalidInput
	}
	p, err := c.resolve(ctx, peer)
	if err != nil {
		return err
	}
	name = filepath.Base(name)
	f, err := uploader.NewUploader(c.client.API()).WithThreads(1).Upload(ctx, uploader.NewUpload(name, io.LimitReader(src, size), size))
	if err != nil {
		return rpcError(err)
	}
	if photo {
		_, err = c.sender.To(p).UploadedPhoto(ctx, f)
	} else {
		_, err = c.sender.To(p).Media(ctx, message.UploadedDocument(f).Filename(name).MIME("application/octet-stream"))
	}
	return rpcError(err)
}
func (c *gotdClient) Download(ctx context.Context, peer string, id int, dst io.Writer) error {
	if id <= 0 || dst == nil {
		return ErrInvalidInput
	}
	p, err := c.resolve(ctx, peer)
	if err != nil {
		return err
	}
	var result tg.MessagesMessagesClass
	if ch, ok := p.(*tg.InputPeerChannel); ok {
		result, err = c.client.API().ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash}, ID: []tg.InputMessageClass{&tg.InputMessageID{ID: id}}})
	} else {
		result, err = c.client.API().MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: id}})
	}
	if err != nil {
		return rpcError(err)
	}
	v, ok := result.AsModified()
	if !ok {
		return ErrInvalidInput
	}
	var location tg.InputFileLocationClass
	var dc int
	for _, raw := range v.GetMessages() {
		m, ok := raw.(*tg.Message)
		if !ok || m.ID != id || peerID(m.PeerID) != inputID(p, c.selfID.Load()) {
			continue
		}
		switch media := m.Media.(type) {
		case *tg.MessageMediaDocument:
			d, ok := media.Document.(*tg.Document)
			if !ok {
				continue
			}
			location = &tg.InputDocumentFileLocation{ID: d.ID, AccessHash: d.AccessHash, FileReference: d.FileReference}
			dc = d.DCID
		case *tg.MessageMediaPhoto:
			d, ok := media.Photo.(*tg.Photo)
			if !ok {
				continue
			}
			thumb := ""
			for _, s := range d.Sizes {
				switch s := s.(type) {
				case *tg.PhotoSize:
					thumb = s.Type
				case *tg.PhotoSizeProgressive:
					thumb = s.Type
				}
			}
			if thumb == "" {
				continue
			}
			location = &tg.InputPhotoFileLocation{ID: d.ID, AccessHash: d.AccessHash, FileReference: d.FileReference, ThumbSize: thumb}
			dc = d.DCID
		}
	}
	if location == nil {
		return ErrInvalidInput
	}
	rpc, err := c.client.MediaOnly(ctx, dc, 1)
	if err != nil {
		return rpcError(err)
	}
	defer rpc.Close()
	_, err = downloader.NewDownloader().Download(tg.NewClient(rpc), location).Stream(ctx, dst)
	return rpcError(err)
}
