package service

import (
	"context"
	"fmt"

	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/store"
	"github.com/google/uuid"
)

type GraphService struct {
	Store *store.Store
}

func (g *GraphService) CampaignGraph(ctx context.Context, campaignID uuid.UUID) (*domain.CampaignGraph, error) {
	pubs, err := g.Store.ListPublicationsByCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	links, err := g.Store.ListCrossLinksByCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	graph := &domain.CampaignGraph{}
	for _, p := range pubs {
		label := string(p.Status)
		if p.ExternalURL != "" {
			label = p.ExternalURL
		}
		graph.Nodes = append(graph.Nodes, domain.GraphNode{
			ID:    p.ID.String(),
			Type:  "publication",
			Label: label,
			Meta: map[string]any{
				"status":     p.Status,
				"channel_id": p.ChannelID.String(),
				"url":        p.ExternalURL,
				"sort_order": p.SortOrder,
			},
		})
	}
	for _, l := range links {
		from := ""
		to := l.ToURL
		if l.FromPublicationID != nil {
			from = l.FromPublicationID.String()
		}
		if l.ToPublicationID != nil {
			to = l.ToPublicationID.String()
		} else if l.ToURL != "" {
			// URL node
			nodeID := "url:" + l.ToURL
			graph.Nodes = append(graph.Nodes, domain.GraphNode{
				ID:    nodeID,
				Type:  "url",
				Label: l.ToURL,
			})
			to = nodeID
		}
		if from == "" || to == "" {
			continue
		}
		graph.Edges = append(graph.Edges, domain.GraphEdge{
			ID:     l.ID.String(),
			From:   from,
			To:     to,
			Kind:   l.Kind,
			Weight: 1,
		})
	}

	// Mentions as nodes if campaign-linked
	// (list all workspace mentions filtered in handler optionally)
	_ = fmt.Sprintf
	return graph, nil
}
