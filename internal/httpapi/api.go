package httpapi

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/nawocci/pogu/internal/provider"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/telemetry"
)

const sessionCookie = "pogu_session"

type API struct {
	Service        *service.Service
	ProviderClient *provider.Client
	Telemetry      *telemetry.Recorder
	Monitor        *monitorHub
	Logger         *slog.Logger
	loginSlots     chan struct{}
	loginMu        sync.Mutex
	loginAttempts  map[string]loginAttempt
	setupMu        sync.Mutex
	setupToken     string
	setupActive    bool
}

type loginAttempt struct {
	Count        int
	BlockedUntil time.Time
}

func New(s *service.Service, logger *slog.Logger) *API {
	api := &API{
		Service:        s,
		ProviderClient: provider.NewClient(),
		Telemetry:      telemetry.NewRecorder(s.Store.DB),
		Monitor:        newMonitorHub(),
		Logger:         logger,
		loginSlots:     make(chan struct{}, 4),
		loginAttempts:  make(map[string]loginAttempt),
	}
	if err := api.Telemetry.ReapStale(time.Now()); err != nil && logger != nil {
		logger.Warn("reap stale telemetry", "error", err.Error())
	}
	return api
}

func (a *API) Handler(static http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /v1/chat/completions", a.openAIChat)
	mux.HandleFunc("POST /v1/messages", a.anthropicMessages)
	mux.HandleFunc("GET /v1/models", a.models)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("GET /api/auth/me", a.me)
	mux.HandleFunc("GET /api/auth/setup", a.setupStatus)
	mux.HandleFunc("POST /api/auth/setup", a.completeSetup)
	mux.HandleFunc("POST /api/auth/password", a.changePassword)
	mux.HandleFunc("GET /api/providers", a.providers)
	mux.HandleFunc("POST /api/providers", a.createProvider)
	mux.HandleFunc("GET /api/providers/{id}", a.getProvider)
	mux.HandleFunc("PUT /api/providers/{id}", a.updateProvider)
	mux.HandleFunc("DELETE /api/providers/{id}", a.deleteProvider)
	mux.HandleFunc("POST /api/providers/{id}/test", a.testProvider)
	mux.HandleFunc("POST /api/providers/{id}/sync", a.syncProvider)
	mux.HandleFunc("GET /api/providers/{id}/keys", a.listProviderKeys)
	mux.HandleFunc("POST /api/providers/{id}/keys", a.createProviderKey)
	mux.HandleFunc("POST /api/providers/{id}/keys/import", a.importProviderKeys)
	mux.HandleFunc("PUT /api/providers/{id}/keys/{keyId}", a.updateProviderKey)
	mux.HandleFunc("DELETE /api/providers/{id}/keys/{keyId}", a.deleteProviderKey)
	mux.HandleFunc("POST /api/providers/{id}/keys/{keyId}/primary", a.makeProviderKeyPrimary)
	mux.HandleFunc("POST /api/providers/{id}/keys/{keyId}/test", a.testProviderKey)
	mux.HandleFunc("GET /api/models", a.listModels)
	mux.HandleFunc("POST /api/models", a.createModel)
	mux.HandleFunc("PUT /api/models/{id}", a.updateModel)
	mux.HandleFunc("DELETE /api/models/{id}", a.deleteModel)
	mux.HandleFunc("GET /api/groups", a.listGroups)
	mux.HandleFunc("POST /api/groups", a.createGroup)
	mux.HandleFunc("GET /api/groups/{id}", a.getGroup)
	mux.HandleFunc("PUT /api/groups/{id}", a.updateGroup)
	mux.HandleFunc("DELETE /api/groups/{id}", a.deleteGroup)
	mux.HandleFunc("GET /api/groups/{id}/members", a.listGroupMembers)
	mux.HandleFunc("POST /api/groups/{id}/members", a.addGroupMember)
	mux.HandleFunc("PUT /api/groups/{id}/members/{memberId}", a.updateGroupMember)
	mux.HandleFunc("DELETE /api/groups/{id}/members/{memberId}", a.deleteGroupMember)
	mux.HandleFunc("POST /api/groups/{id}/members/order", a.reorderGroupMembers)
	mux.HandleFunc("POST /api/groups/{id}/selection", a.setGroupSelection)
	mux.HandleFunc("GET /api/keys", a.listKeys)
	mux.HandleFunc("POST /api/keys", a.createKey)
	mux.HandleFunc("POST /api/keys/{id}/revoke", a.revokeKey)
	mux.HandleFunc("GET /api/settings/prompt", a.getGlobalPrompt)
	mux.HandleFunc("PUT /api/settings/prompt", a.updateGlobalPrompt)
	mux.HandleFunc("GET /api/monitoring/summary", a.monitoringSummary)
	mux.HandleFunc("GET /api/monitoring/timeseries", a.monitoringTimeseries)
	mux.HandleFunc("GET /api/monitoring/requests", a.monitoringRequests)
	mux.HandleFunc("GET /api/monitoring/events", a.monitoringEvents)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		jsonError(w, http.StatusNotFound, "not found")
	})
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		jsonError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", static)
	return logging(a.authMiddleware(mux), a.Logger)
}

func logging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if logger != nil {
			logger.Info("request", "method", r.Method, "path", r.URL.Path)
		}
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	jsonWrite(w, http.StatusOK, map[string]any{"status": "ok"})
}
