package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nawocci/pogu/internal/provider"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/telemetry"
)

type routeTarget struct {
	Route service.Route
	Keys  []service.ProviderKey
}

func (a *API) openAIChat(w http.ResponseWriter, r *http.Request) {
	a.gateway(w, r, "openai")
}

func (a *API) anthropicMessages(w http.ResponseWriter, r *http.Request) {
	a.gateway(w, r, "anthropic")
}

func isFailoverStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests || status == http.StatusPaymentRequired
}

func (a *API) resolveTargets(ctx context.Context, model, protocol string) ([]routeTarget, service.Group, bool, error) {
	if strings.Contains(model, "/") {
		prefix, rest, ok := strings.Cut(model, "/")
		if !ok || prefix == "" || rest == "" || !service.ValidProviderPrefix(prefix) {
			return nil, service.Group{}, false, errors.New("model must be a group name or a provider-prefix/model reference")
		}
		route, err := a.Service.ResolveRoute(ctx, model)
		if err != nil {
			return nil, service.Group{}, false, err
		}
		keys, err := a.Service.EligibleKeys(ctx, route.Provider.ID)
		if err != nil {
			return nil, service.Group{}, false, err
		}
		if len(keys) == 0 {
			return nil, service.Group{}, false, service.ErrNoCredentials
		}
		return []routeTarget{{Route: route, Keys: keys}}, service.Group{}, false, nil
	}
	group, candidates, err := a.Service.ResolveGroupTargets(ctx, model, protocol)
	if err != nil {
		return nil, group, false, err
	}
	targets := make([]routeTarget, len(candidates))
	for i, c := range candidates {
		targets[i] = routeTarget{Route: c.Route, Keys: c.Keys}
	}
	return targets, group, true, nil
}

func (a *API) writeRouteError(w http.ResponseWriter, protocol string, err error) {
	switch {
	case errors.Is(err, service.ErrNoGroupTargets):
		writeProtocolError(w, protocol, http.StatusServiceUnavailable, "group has no available targets", upstreamErrorType(protocol))
	case errors.Is(err, service.ErrNoCredentials):
		writeProtocolError(w, protocol, http.StatusBadGateway, "no enabled provider credentials", upstreamErrorType(protocol))
	case strings.HasPrefix(err.Error(), "model must be"):
		writeProtocolError(w, protocol, http.StatusBadRequest, err.Error(), "invalid_request_error")
	default:
		writeProtocolError(w, protocol, http.StatusNotFound, "unknown model or group", "not_found_error")
	}
}

func (a *API) gateway(w http.ResponseWriter, r *http.Request, protocol string) {
	key, ok := a.authenticateAI(w, r, protocol)
	if !ok {
		return
	}
	body, err := readBoundedBody(r, 10<<20)
	if err != nil {
		writeProtocolError(w, protocol, http.StatusRequestEntityTooLarge, "request body exceeds size limit", "invalid_request_error")
		return
	}
	rawBody := body
	if protocol == "anthropic" {
		var pre struct {
			Stream bool `json:"stream"`
		}
		if err := json.Unmarshal(body, &pre); err != nil {
			writeProtocolError(w, protocol, http.StatusBadRequest, "request must contain a valid model", "invalid_request_error")
			return
		}
		translated, err := provider.TranslateAnthropicToChat(body, pre.Stream)
		if err != nil {
			writeProtocolError(w, protocol, http.StatusBadRequest, "request must contain valid messages", "invalid_request_error")
			return
		}
		body = translated
	}
	var envelope struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Model == "" {
		writeProtocolError(w, protocol, http.StatusBadRequest, "request must contain a valid model", "invalid_request_error")
		return
	}
	targets, group, isGroup, err := a.resolveTargets(r.Context(), envelope.Model, protocol)
	if err != nil {
		a.writeRouteError(w, protocol, err)
		return
	}

	rec := a.newGatewayTelemetry(w, r, protocol, key, envelope.Model, targets[0].Route, envelope.Stream, targets[0].Keys[0].ID)
	if isGroup {
		rec.recordGroupResolution(group, targets[0].Route.Model.PublicID)
	} else {
		rec.recordDirectRoute()
	}

	if text, enabled, err := a.Service.GetGlobalPrompt(r.Context()); err == nil {
		if prompt := service.ActivePrompt(text, enabled); prompt != "" {
			body = provider.InjectChatPrompt(body, prompt)
			if protocol == "anthropic" {
				rawBody = provider.InjectAnthropicPrompt(rawBody, prompt)
			}
		}
	}

	var lastProxyErr error
	lastHTTPStatus := http.StatusBadGateway
	attemptNum := 0

	for _, target := range targets {
		body = replaceModel(body, target.Route.Model.Name)
		rawBody = replaceModel(rawBody, target.Route.Model.Name)
		scheme := string(service.EffectiveScheme(target.Route.Provider, target.Route.Model))
		upstreamPath := provider.UpstreamPathForScheme(scheme)
		for _, pKey := range target.Keys {
			attemptNum++
			attemptStart := time.Now()

			if attemptNum > 1 {
				rec.recordAttemptStart(attemptNum, target.Route, pKey.ID, attemptStart)
			}

			secret, err := a.Service.ProviderKeySecret(r.Context(), pKey.ID)
			if err != nil {
				lastProxyErr = err
				rec.recordAttemptFinish(attemptNum, nil, false, telemetry.ErrInternal, telemetry.TokenUsage{}, attemptStart, time.Now())
				continue
			}

			_ = a.Service.TouchProviderKeyUsed(r.Context(), pKey.ID)

			tracked := &statusWriter{ResponseWriter: w}
			var usageBody bytes.Buffer
			var collector *provider.SSEUsageCollector
			if envelope.Stream {
				collector = provider.NewSSEUsageCollector(protocol)
				collector.OnFirst = func() {
					first, _ := collector.FirstByte()
					served, _ := collector.FirstModel()
					rec.noteFirstByte(first, served)
				}
			}

			var proxyErr error
			if envelope.Stream {
				proxyErr = a.proxyStream(r.Context(), protocol, scheme, target.Route.Provider, secret, upstreamPath, body, rawBody, tracked, collector)
			} else {
				proxyErr = a.proxyUnary(r.Context(), protocol, scheme, target.Route.Provider, secret, upstreamPath, body, rawBody, tracked, &usageBody)
			}

			completedAt := time.Now()
			lastProxyErr = proxyErr
			if collector != nil {
				if first, ok := collector.FirstByte(); ok {
					served, _ := collector.FirstModel()
					rec.noteFirstByte(first, served)
				}
			}

			if proxyErr == nil {
				rec.finishSuccess(attemptNum, tracked, collector, &usageBody, attemptStart, completedAt)
				return
			}

			if tracked.committed || errors.Is(proxyErr, context.Canceled) || errors.Is(proxyErr, r.Context().Err()) {
				rec.finishCommittedOrCanceled(attemptNum, proxyErr, tracked, collector, &usageBody, attemptStart, completedAt)
				return
			}

			var upstreamErr *provider.UpstreamError
			if errors.As(proxyErr, &upstreamErr) && isFailoverStatus(upstreamErr.Status) {
				category := classifyErrorCategory(proxyErr)
				statusVal := upstreamErr.Status
				rec.recordAttemptFinish(attemptNum, &statusVal, false, category, telemetry.TokenUsage{}, attemptStart, completedAt)
				lastHTTPStatus = statusVal
				continue
			}

			category := classifyErrorCategory(proxyErr)
			statusVal := clientStatusForUpstream(proxyErr, http.StatusBadGateway)
			rec.recordAttemptFinish(attemptNum, &statusVal, false, category, telemetry.TokenUsage{}, attemptStart, completedAt)
			rec.finishRequest(statusVal, category, telemetry.TokenUsage{}, completedAt)
			writeProtocolError(w, protocol, statusVal, provider.StatusMessage(proxyErr), upstreamErrorType(protocol))
			return
		}
	}

	category := classifyErrorCategory(lastProxyErr)
	rec.finishRequest(lastHTTPStatus, category, telemetry.TokenUsage{}, time.Now())
	status := lastHTTPStatus
	if len(targets) > 1 && status == http.StatusBadGateway {
		status = http.StatusServiceUnavailable
	}
	writeProtocolError(w, protocol, status, provider.StatusMessage(lastProxyErr), upstreamErrorType(protocol))
}

func (a *API) proxyStream(ctx context.Context, client, scheme string, p service.Provider, secret, path string, body, rawBody []byte, w http.ResponseWriter, collector *provider.SSEUsageCollector) error {
	c := a.ProviderClient
	switch {
	case scheme == string(service.SchemeOpenAIResponses) && client == "anthropic":
		return provider.ProxyResponsesInletStream(ctx, c, p, secret, path, rawBody, w, collector)
	case scheme == string(service.SchemeOpenAIResponses):
		return provider.ProxyResponsesStream(ctx, c, p, secret, path, body, w, collector)
	case scheme == string(service.SchemeAnthropic) && client == "anthropic":
		return provider.ProxyStream(ctx, c, p, secret, path, rawBody, w, collector)
	case scheme == string(service.SchemeAnthropic):
		return provider.ProxyAnthropicStream(ctx, c, p, secret, path, body, w, collector)
	case client == "anthropic":
		return provider.ProxyChatInletStream(ctx, c, p, secret, path, rawBody, w, collector)
	default:
		return provider.ProxyStream(ctx, c, p, secret, path, body, w, collector)
	}
}

func (a *API) proxyUnary(ctx context.Context, client, scheme string, p service.Provider, secret, path string, body, rawBody []byte, w http.ResponseWriter, tee io.Writer) error {
	c := a.ProviderClient
	switch {
	case scheme == string(service.SchemeOpenAIResponses) && client == "anthropic":
		return provider.ProxyResponsesInletUnary(ctx, c, p, secret, path, rawBody, w, tee)
	case scheme == string(service.SchemeOpenAIResponses):
		return provider.ProxyResponsesUnary(ctx, c, p, secret, path, body, w, tee)
	case scheme == string(service.SchemeAnthropic) && client == "anthropic":
		return provider.ProxyUnary(ctx, c, p, secret, path, rawBody, w, tee)
	case scheme == string(service.SchemeAnthropic):
		return provider.ProxyAnthropicUnary(ctx, c, p, secret, path, body, w, tee)
	case client == "anthropic":
		return provider.ProxyChatInletUnary(ctx, c, p, secret, path, rawBody, w, tee)
	default:
		return provider.ProxyUnary(ctx, c, p, secret, path, body, w, tee)
	}
}

func (a *API) authenticateAI(w http.ResponseWriter, r *http.Request, protocol string) (service.APIKey, bool) {
	secret := ""
	if protocol == "anthropic" {
		secret = strings.TrimSpace(r.Header.Get("x-api-key"))
		if secret == "" {
			secret = extractBearer(r)
		}
	} else {
		secret = extractBearer(r)
		if secret == "" {
			secret = strings.TrimSpace(r.Header.Get("x-api-key"))
		}
	}
	if secret == "" {
		writeProtocolError(w, protocol, http.StatusUnauthorized, "missing API key", "authentication_error")
		return service.APIKey{}, false
	}
	key, err := a.Service.AuthenticateAPIKey(r.Context(), secret)
	if err != nil {
		writeProtocolError(w, protocol, http.StatusUnauthorized, "invalid API key", "authentication_error")
		return service.APIKey{}, false
	}
	return key, true
}

func (a *API) models(w http.ResponseWriter, r *http.Request) {
	protocol := "openai"
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "anthropic") || r.Header.Get("anthropic-version") != "" {
		protocol = "anthropic"
	}
	if _, ok := a.authenticateAI(w, r, protocol); !ok {
		return
	}
	providers, err := a.Service.ListProviders(r.Context())
	if err != nil {
		writeProtocolError(w, protocol, http.StatusInternalServerError, "could not list models", internalErrorType(protocol))
		return
	}
	active := make(map[int64]bool, len(providers))
	for _, p := range providers {
		active[p.ID] = p.Enabled
	}
	models, err := a.Service.ListModels(r.Context())
	if err != nil {
		writeProtocolError(w, protocol, http.StatusInternalServerError, "could not list models", internalErrorType(protocol))
		return
	}
	entries := make([]map[string]any, 0, len(models))
	for _, model := range models {
		if !model.Enabled || !active[model.ProviderID] {
			continue
		}
		if protocol == "anthropic" {
			entries = append(entries, map[string]any{
				"id":           model.PublicID,
				"type":         "model",
				"display_name": model.PublicID,
				"created_at":   model.CreatedAt.UTC().Format(time.RFC3339),
			})
		} else {
			entries = append(entries, map[string]any{
				"id":       model.PublicID,
				"object":   "model",
				"created":  model.CreatedAt.Unix(),
				"owned_by": "pogu",
			})
		}
	}
	groups, err := a.Service.ListGroups(r.Context())
	if err != nil {
		writeProtocolError(w, protocol, http.StatusInternalServerError, "could not list models", internalErrorType(protocol))
		return
	}
	for _, group := range groups {
		if !group.Enabled || group.MemberCount == 0 {
			continue
		}
		if protocol == "anthropic" {
			entries = append(entries, map[string]any{
				"id":           group.Name,
				"type":         "model",
				"display_name": group.Name,
				"created_at":   group.CreatedAt.UTC().Format(time.RFC3339),
			})
		} else {
			entries = append(entries, map[string]any{
				"id":       group.Name,
				"object":   "model",
				"created":  group.CreatedAt.Unix(),
				"owned_by": "pogu",
			})
		}
	}
	if protocol == "anthropic" {
		jsonWrite(w, http.StatusOK, map[string]any{
			"data":     entries,
			"first_id": nil,
			"last_id":  nil,
			"has_more": false,
		})
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"object": "list", "data": entries})
}

func replaceModel(body []byte, model string) []byte {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	object["model"] = model
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}

func extractBearer(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if len(value) < 7 || !strings.EqualFold(value[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

type statusWriter struct {
	http.ResponseWriter
	committed bool
	status    int
}

func (w *statusWriter) WriteHeader(status int) {
	if !w.committed {
		w.committed = true
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *statusWriter) Flush() {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

var _ http.Flusher = (*statusWriter)(nil)

func readBoundedBody(r *http.Request, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("request body exceeds size limit")
	}
	return body, nil
}
