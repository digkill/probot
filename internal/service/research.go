package service

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/crawler"
	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/store"
	"github.com/google/uuid"
)

type Researcher struct {
	Store  *store.Store
	AI     *ai.Client
	Runner *crawler.Runner
	HTTP   *http.Client
}

type ResearchResult struct {
	Hits      int            `json:"hits"`
	Mentions  int            `json:"mentions"`
	Agents    []string       `json:"agents"`
	Errors    []string       `json:"errors,omitempty"`
	Providers []string       `json:"providers"`
}

func (r *Researcher) Run(ctx context.Context, workspaceID, brandID uuid.UUID) (ResearchResult, error) {
	out := ResearchResult{}
	brand, err := r.Store.GetBrand(ctx, brandID)
	if err != nil {
		return out, err
	}
	watch := crawler.BrandWatchQuery(brand.Name, brand.Slug, brand.CanonicalURL)
	httpClient := r.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	hits := crawler.SearchWeb(ctx, httpClient, brand.Name, brand.CanonicalURL)
	out.Hits = len(hits)

	agents, err := r.Store.ListAgents(ctx, workspaceID)
	if err != nil {
		return out, err
	}
	var researchers []domain.AIAgent
	for _, a := range agents {
		if !a.Enabled {
			continue
		}
		if a.Role == domain.AgentResearch || a.Role == domain.AgentReputation {
			researchers = append(researchers, a)
		}
	}
	if len(researchers) == 0 {
		out.Errors = append(out.Errors, "no research/reputation agents configured")
		return out, nil
	}

	bid := brand.ID
	prompt := ai.ResearchPrompt(brand.Name, brand.CanonicalURL, watch)
	for _, agent := range researchers {
		out.Agents = append(out.Agents, agent.Name+"/"+agent.Provider)
		input := ai.RunInput{
			Prompt:     prompt,
			LiveSearch: true,
			Context: map[string]any{
				"brand": map[string]string{
					"name": brand.Name, "slug": brand.Slug, "canonical_url": brand.CanonicalURL,
				},
				"web_hits": hits,
			},
		}
		res, err := r.AI.Run(ctx, agent, input)
		if err != nil {
			out.Errors = append(out.Errors, agent.Name+": "+err.Error())
			continue
		}
		findings := ai.ParseFindings(res.Text)
		source := "AI " + ai.NormalizeProvider(agent.Provider)
		out.Providers = append(out.Providers, ai.NormalizeProvider(agent.Provider))
		for _, f := range findings {
			snippet := f.Snippet
			if snippet == "" {
				snippet = f.Title
			}
			n, err := r.Runner.IngestPublic(ctx, workspaceID, &bid, source, f.URL, f.Title, snippet, f.Author, watch, "mentions", time.Time{})
			if err != nil {
				log.Printf("research ingest: %v", err)
				continue
			}
			out.Mentions += n
		}
	}
	return out, nil
}
