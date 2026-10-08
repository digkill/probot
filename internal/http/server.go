package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/digkill/probot/internal/ai"
	"github.com/digkill/probot/internal/auth"
	"github.com/digkill/probot/internal/config"
	"github.com/digkill/probot/internal/mail"
	"github.com/digkill/probot/internal/platforms"
	"github.com/digkill/probot/internal/platforms/generic"
	"github.com/digkill/probot/internal/service"
	"github.com/digkill/probot/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type Server struct {
	cfg       *config.Config
	store     *store.Store
	registry  *platforms.Registry
	publisher *service.Publisher
	graph     *service.GraphService
	ai        *ai.Client
	asynq     *asynq.Client
	generic   *generic.Publisher
	telegram  *service.TelegramService
	mailer    mail.Sender
}

func NewServer(
	cfg *config.Config,
	st *store.Store,
	registry *platforms.Registry,
	asynqClient *asynq.Client,
	telegramService *service.TelegramService,
) *Server {
	gen := generic.NewPublisher()
	pub := &service.Publisher{
		Store:         st,
		Registry:      registry,
		Generic:       gen,
		EncKey:        []byte(cfg.EncryptionKey),
		PublicBaseURL: cfg.PublicBaseURL,
	}
	return &Server{
		cfg:       cfg,
		telegram:  telegramService,
		store:     st,
		registry:  registry,
		publisher: pub,
		graph:     &service.GraphService{Store: st},
		ai:        ai.NewClient(cfg),
		asynq:     asynqClient,
		generic:   gen,
		mailer: mail.New(mail.Config{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser,
			Password: cfg.SMTPPassword, From: cfg.SMTPFrom,
		}, log.Default()),
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(safeAccessLogger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:3000", "http://127.0.0.1:3000", "http://localhost", "http://localhost:80", "*"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/r/{code}", s.handleRedirectShortLink)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/password/forgot", s.handleForgotPassword)
		r.Post("/auth/password/reset", s.handleResetPassword)
		r.Get("/playbooks", s.handleListPlaybooks)
		r.Get("/contacts-export/{token}", s.handleContactExport)

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Get("/me/workspaces", s.handleListWorkspaces)

			r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
				r.Use(s.workspaceMiddleware)
				r.Route("/telegram", s.telegramRoutes)
				r.Route("/contacts", s.contactRoutes)
				r.Get("/brands", s.handleListBrands)
				r.Post("/brands", s.handleCreateBrand)
				r.Patch("/brands/{brandID}", s.handleUpdateBrand)
				r.Post("/brands/{brandID}/review-watch", s.handleSeedReviewWatch)
				r.Post("/brands/{brandID}/ai-research", s.handleBrandAIResearch)

				r.Get("/analytics/summary", s.handleAnalyticsSummary)
				r.Post("/analytics/advise", s.handleAnalyticsAdvise)
				r.Post("/analytics/poll-stats", s.handleTriggerStatsPoll)

				r.Get("/campaigns", s.handleListCampaigns)
				r.Post("/campaigns", s.handleCreateCampaign)
				r.Get("/campaigns/{campaignID}/graph", s.handleCampaignGraph)
				r.Post("/campaigns/{campaignID}/apply-playbook", s.handleApplyPlaybook)

				r.Get("/short-links", s.handleListShortLinks)
				r.Post("/short-links", s.handleCreateShortLink)

				r.Get("/content", s.handleListContent)
				r.Post("/content", s.handleCreateContent)

				r.Get("/channels", s.handleListChannels)
				r.Post("/channels", s.handleCreateChannel)
				r.Patch("/channels/{channelID}/health", s.handleSetChannelHealth)

				r.Get("/publications", s.handleListPublications)
				r.Post("/publications", s.handleCreatePublication)
				r.Post("/publications/{publicationID}/enqueue", s.handleEnqueuePublication)
				r.Post("/publications/{publicationID}/confirm", s.handleConfirmPublication)

				r.Get("/platforms", s.handleListPlatforms)
				r.Get("/custom-platforms", s.handleListCustomPlatforms)
				r.Post("/custom-platforms", s.handleCreateCustomPlatform)

				r.Get("/agents", s.handleListAgents)
				r.Post("/agents", s.handleCreateAgent)
				r.Post("/agents/seed-research", s.handleSeedResearchAgents)
				r.Patch("/agents/{agentID}", s.handleUpdateAgent)
				r.Delete("/agents/{agentID}", s.handleDeleteAgent)
				r.Post("/agents/{agentID}/run", s.handleRunAgent)
				r.Get("/ai/providers", s.handleListAIProviders)

				r.Get("/mentions/summary", s.handleMentionInbox)
				r.Get("/mentions", s.handleListMentions)
				r.Post("/mentions", s.handleCreateMention)
				r.Post("/mentions/{mentionID}/draft-reply", s.handleMentionDraftReply)
				r.Post("/mentions/{mentionID}/objection", s.handleMentionObjection)
				r.Post("/mentions/{mentionID}/status", s.handleMentionStatus)
				r.Delete("/mentions/{mentionID}", s.handleDeleteMention)
				r.Post("/mentions/delete-false-positives", s.handleDeleteFalsePositiveMentions)
				r.Post("/mentions/{mentionID}/engage", s.handleEngagementFromMention)

				r.Get("/engagement/tasks", s.handleListEngagement)
				r.Post("/engagement/tasks", s.handleCreateEngagement)
				r.Post("/engagement/tasks/{taskID}/draft", s.handleDraftEngagement)
				r.Post("/engagement/tasks/{taskID}/approve", s.handleApproveEngagement)
				r.Post("/engagement/tasks/{taskID}/skip", s.handleSkipEngagement)
				r.Post("/engagement/tasks/{taskID}/confirm", s.handleConfirmEngagementManual)
				r.Get("/engagement/karma", s.handleKarmaSummary)
				r.Get("/engagement/partners", s.handleListPartners)
				r.Post("/engagement/partners", s.handleCreatePartner)
				r.Patch("/engagement/partners/{partnerID}", s.handleUpdatePartner)

				r.Get("/crawl-sources", s.handleListCrawlSources)
				r.Post("/crawl-sources", s.handleCreateCrawlSource)
				r.Post("/crawl-sources/{sourceID}/run", s.handleRunCrawlSource)
				r.Post("/crawl-sources/run-due", s.handleTriggerCrawlDue)
			})
		})
	})

	return r
}

type ctxKey string

const (
	ctxUserID      ctxKey = "userID"
	ctxWorkspaceID ctxKey = "workspaceID"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "Please sign in.")
			return
		}
		claims, err := auth.Parse(s.cfg.JWTSecret, strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "Session expired. Please sign in again.")
			return
		}
		ctx := contextWith(r.Context(), ctxUserID, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) workspaceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid workspace id")
			return
		}
		userID := mustUserID(r)
		if _, err := s.store.UserRole(r.Context(), wsID, userID); err != nil {
			writeErr(w, http.StatusForbidden, "You don't have access to this workspace.")
			return
		}
		ctx := contextWith(r.Context(), ctxWorkspaceID, wsID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
