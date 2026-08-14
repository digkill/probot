package service

import (
	"context"
	"encoding/json"
	"log"

	"github.com/digkill/probot/internal/auth"
	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/store"
	"github.com/google/uuid"
)

type StatsPoller struct {
	Store    *store.Store
	Registry *platforms.Registry
	EncKey   []byte
}

func (p *StatsPoller) PollAll(ctx context.Context) (int, error) {
	pubs, err := p.Store.ListLivePublications(ctx, 200)
	if err != nil {
		return 0, err
	}
	ok := 0
	for _, pub := range pubs {
		if err := p.FetchOne(ctx, pub.ID); err != nil {
			log.Printf("stats %s: %v", pub.ID, err)
			continue
		}
		ok++
	}
	return ok, nil
}

func (p *StatsPoller) FetchOne(ctx context.Context, publicationID uuid.UUID) error {
	pub, err := p.Store.GetPublication(ctx, publicationID)
	if err != nil {
		return err
	}
	if pub.ExternalID == "" {
		return nil
	}
	ch, credBlob, err := p.Store.GetChannel(ctx, pub.ChannelID)
	if err != nil {
		return err
	}
	if ch.PlatformDefID == nil {
		return nil
	}
	def, err := p.Store.GetPlatformDefinition(ctx, *ch.PlatformDefID)
	if err != nil {
		return err
	}
	adapter, ok := p.Registry.Get(def.Slug)
	if !ok {
		return nil
	}
	creds, err := auth.DecryptCredentials(p.EncKey, credBlob)
	if err != nil {
		return err
	}
	if creds.ExternalRef == "" {
		creds.ExternalRef = ch.ExternalRef
	}
	stats, err := adapter.FetchStats(ctx, creds, pub.ExternalID)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(stats.Raw)
	if raw == nil {
		raw = json.RawMessage(`{}`)
	}
	return p.Store.InsertMetricSnapshot(ctx, &store.MetricSnapshot{
		PublicationID: pub.ID,
		Reach:         stats.Reach,
		Likes:         stats.Likes,
		Comments:      stats.Comments,
		Shares:        stats.Shares,
		Clicks:        stats.Clicks,
		Raw:           raw,
	})
}
