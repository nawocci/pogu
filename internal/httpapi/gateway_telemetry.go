package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/nawocci/pogu/internal/provider"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/telemetry"
)

type gatewayTelemetry struct {
	recorder   *telemetry.Recorder
	logger     *slog.Logger
	hub        *monitorHub
	id         string
	protocol   string
	startedAt  time.Time
	streaming  bool
	route      service.Route
	public     string
	keyName    string
	upstreamMS int64
	ttftMS     *int64
	served     *string
}

func (g *gatewayTelemetry) recordGroupResolution(group service.Group, resolvedModel string) {
	if g == nil || g.id == "" || g.recorder == nil {
		return
	}
	if err := g.recorder.RecordResolution(g.id, &group.ID, &group.Name, resolvedModel); err != nil {
		g.logFailure("record group resolution", err)
	}
	g.hub.publishRouted(&monitorRouted{
		RequestID:     g.id,
		Provider:      g.route.Provider.Name,
		UpstreamModel: g.route.Model.Name,
		ResolvedModel: resolvedModel,
		GroupName:     group.Name,
	})
}

func (g *gatewayTelemetry) recordDirectRoute() {
	if g == nil || g.id == "" {
		return
	}
	g.hub.publishRouted(&monitorRouted{
		RequestID:     g.id,
		Provider:      g.route.Provider.Name,
		UpstreamModel: g.route.Model.Name,
	})
}

func (a *API) newGatewayTelemetry(w http.ResponseWriter, r *http.Request, protocol string, key service.APIKey, publicModel string, route service.Route, streaming bool, initialKeyID int64) *gatewayTelemetry {
	g := &gatewayTelemetry{
		recorder:  a.Telemetry,
		logger:    a.Logger,
		hub:       a.Monitor,
		protocol:  protocol,
		startedAt: time.Now(),
		streaming: streaming,
		route:     route,
		public:    publicModel,
		keyName:   key.Name,
	}
	id, err := telemetry.NewID()
	if err != nil {
		g.logFailure("generate request id", err)
		return g
	}
	g.id = id
	w.Header().Set("X-Request-Id", id)

	req := &telemetry.Request{
		ID:            id,
		Protocol:      protocol,
		PublicModel:   publicModel,
		Streaming:     streaming,
		ClientKeyID:   &key.ID,
		ClientKeyName: &key.Name,
		Status:        telemetry.StatusInProgress,
		StartedAt:     g.startedAt,
	}
	attemptStart := time.Now()
	attempt := &telemetry.Attempt{
		RequestID:      id,
		Number:         1,
		ProviderID:     &route.Provider.ID,
		ProviderName:   route.Provider.Name,
		ProviderPrefix: route.Provider.Prefix,
		ProviderType:   string(route.Provider.Type),
		ModelID:        &route.Model.ID,
		UpstreamModel:  route.Model.Name,
		CredentialID:   &initialKeyID,
		StartedAt:      attemptStart,
		Success:        false,
	}
	if err := g.recorder.StartRequest(req, attempt); err != nil {
		g.logFailure("record request start", err)
	}
	g.hub.publishStarted(&monitorStarted{
		RequestID: g.id,
		TS:        nowString(g.startedAt),
		KeyID:     &key.ID,
		KeyName:   key.Name,
		Model:     publicModel,
		Protocol:  protocol,
		Stream:    streaming,
	})
	return g
}

func (g *gatewayTelemetry) recordAttemptStart(attemptNum int, route service.Route, keyID int64, attemptStart time.Time) {
	if g == nil || g.id == "" || g.recorder == nil {
		return
	}
	attempt := &telemetry.Attempt{
		RequestID:      g.id,
		Number:         attemptNum,
		ProviderID:     &route.Provider.ID,
		ProviderName:   route.Provider.Name,
		ProviderPrefix: route.Provider.Prefix,
		ProviderType:   string(route.Provider.Type),
		ModelID:        &route.Model.ID,
		UpstreamModel:  route.Model.Name,
		CredentialID:   &keyID,
		StartedAt:      attemptStart,
		Success:        false,
	}
	if err := g.recorder.StartAttempt(attempt); err != nil {
		g.logFailure("record attempt start", err)
	}
	g.hub.publishRouted(&monitorRouted{
		RequestID:     g.id,
		Provider:      route.Provider.Name,
		UpstreamModel: route.Model.Name,
	})
}

func (g *gatewayTelemetry) recordAttemptFinish(attemptNum int, httpStatus *int, success bool, category telemetry.ErrorCategory, usage telemetry.TokenUsage, startedAt, completedAt time.Time) {
	if g == nil || g.id == "" || g.recorder == nil {
		return
	}
	if ms := completedAt.Sub(startedAt).Milliseconds(); ms > 0 {
		g.upstreamMS += ms
	}
	if err := g.recorder.FinishAttemptWithTiming(g.id, attemptNum, httpStatus, success, category, usage, startedAt, completedAt); err != nil {
		g.logFailure("record attempt finish", err)
	}
}

func (g *gatewayTelemetry) noteFirstByte(first time.Time, served string) {
	if g == nil || g.id == "" || g.ttftMS != nil {
		return
	}
	ms := first.Sub(g.startedAt).Milliseconds()
	if ms < 0 {
		ms = 0
	}
	g.ttftMS = &ms
	if served != "" {
		g.served = &served
	}
	g.hub.publishStreaming(&monitorStreaming{RequestID: g.id, TTFTMS: ms, ServedModel: served})
}

func (g *gatewayTelemetry) servedFrom(collector *provider.SSEUsageCollector, usageBody *bytes.Buffer) *string {
	if g.served != nil {
		return g.served
	}
	if collector != nil {
		if model, ok := collector.FirstModel(); ok {
			g.served = &model
			return g.served
		}
		return g.served
	}
	if usageBody != nil {
		if model, ok := provider.ExtractUpstreamModel(usageBody.Bytes()); ok {
			g.served = &model
		}
	}
	return g.served
}

func (g *gatewayTelemetry) publishFinished() {
	if g == nil || g.id == "" || g.recorder == nil {
		return
	}
	rec, err := g.recorder.GetRecord(context.Background(), g.id)
	if err != nil {
		g.logFailure("read finished record", err)
		return
	}
	g.hub.publishFinished(rec)
}

func (g *gatewayTelemetry) finishSuccess(attemptNum int, tracked *statusWriter, collector *provider.SSEUsageCollector, usageBody *bytes.Buffer, attemptStart, completedAt time.Time) {
	if g == nil || g.id == "" || g.recorder == nil {
		return
	}
	usage := telemetry.TokenUsage{}
	if collector != nil {
		usage = collector.Usage()
	} else if usageBody != nil {
		usage = provider.ExtractUnaryUsage(g.protocol, usageBody.Bytes())
	}
	status200 := http.StatusOK
	if tracked != nil && tracked.committed && tracked.status >= 200 && tracked.status <= 299 {
		status200 = tracked.status
	}
	g.recordAttemptFinish(attemptNum, &status200, true, telemetry.ErrNone, usage, attemptStart, completedAt)
	if err := g.recorder.FinishRequestWithTiming(g.id, telemetry.StatusSuccess, &status200, telemetry.ErrNone, usage, g.ttftMS, &g.upstreamMS, g.servedFrom(collector, usageBody), g.startedAt, completedAt); err != nil {
		g.logFailure("record request finish", err)
	}
	g.publishFinished()
}

func (g *gatewayTelemetry) finishCommittedOrCanceled(attemptNum int, proxyErr error, tracked *statusWriter, collector *provider.SSEUsageCollector, usageBody *bytes.Buffer, attemptStart, completedAt time.Time) {
	if g == nil || g.id == "" || g.recorder == nil {
		return
	}
	category, status, success := g.classify(proxyErr)
	var httpStatus *int
	clientStatus := http.StatusInternalServerError
	if tracked.committed {
		clientStatus = tracked.status
		httpStatus = &clientStatus
	}
	if success && httpStatus != nil && (*httpStatus < 200 || *httpStatus > 299) {
		category, status, success = telemetry.ErrUpstream, telemetry.StatusError, false
	}
	if !success && (httpStatus == nil || category == telemetry.ErrAuth || category == telemetry.ErrRateLimit ||
		category == telemetry.ErrUpstream || category == telemetry.ErrServer) {
		mapped := clientStatusForUpstream(proxyErr, clientStatus)
		httpStatus = &mapped
	}

	usage := telemetry.TokenUsage{}
	if collector != nil {
		usage = collector.Usage()
	} else if usageBody != nil {
		usage = provider.ExtractUnaryUsage(g.protocol, usageBody.Bytes())
	}

	g.recordAttemptFinish(attemptNum, httpStatus, success, category, usage, attemptStart, completedAt)
	if err := g.recorder.FinishRequestWithTiming(g.id, status, httpStatus, category, usage, g.ttftMS, &g.upstreamMS, g.servedFrom(collector, usageBody), g.startedAt, completedAt); err != nil {
		g.logFailure("record request finish", err)
	}
	g.publishFinished()
}

func (g *gatewayTelemetry) finishRequest(httpStatus int, category telemetry.ErrorCategory, usage telemetry.TokenUsage, completedAt time.Time) {
	if g == nil || g.id == "" || g.recorder == nil {
		return
	}
	if err := g.recorder.FinishRequestWithTiming(g.id, telemetry.StatusError, &httpStatus, category, usage, g.ttftMS, &g.upstreamMS, g.served, g.startedAt, completedAt); err != nil {
		g.logFailure("record request finish", err)
	}
	g.publishFinished()
}

func (g *gatewayTelemetry) classify(proxyErr error) (telemetry.ErrorCategory, string, bool) {
	if proxyErr == nil {
		return telemetry.ErrNone, telemetry.StatusSuccess, true
	}
	if errors.Is(proxyErr, context.Canceled) || errors.Is(proxyErr, context.DeadlineExceeded) {
		return telemetry.ErrCanceled, telemetry.StatusError, false
	}
	var upstreamErr *provider.UpstreamError
	if errors.As(proxyErr, &upstreamErr) {
		switch {
		case upstreamErr.Status == http.StatusUnauthorized || upstreamErr.Status == http.StatusForbidden:
			return telemetry.ErrAuth, telemetry.StatusError, false
		case upstreamErr.Status == http.StatusTooManyRequests:
			return telemetry.ErrRateLimit, telemetry.StatusError, false
		case upstreamErr.Status >= 500 && upstreamErr.Status <= 599:
			return telemetry.ErrServer, telemetry.StatusError, false
		case upstreamErr.Status >= 400 && upstreamErr.Status <= 499:
			return telemetry.ErrUpstream, telemetry.StatusError, false
		default:
			return telemetry.ErrConnection, telemetry.StatusError, false
		}
	}
	if errors.Is(proxyErr, context.DeadlineExceeded) {
		return telemetry.ErrTimeout, telemetry.StatusError, false
	}
	return telemetry.ErrConnection, telemetry.StatusError, false
}

func classifyErrorCategory(err error) telemetry.ErrorCategory {
	if err == nil {
		return telemetry.ErrNone
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return telemetry.ErrCanceled
	}
	var upstreamErr *provider.UpstreamError
	if errors.As(err, &upstreamErr) {
		switch {
		case upstreamErr.Status == http.StatusUnauthorized || upstreamErr.Status == http.StatusForbidden:
			return telemetry.ErrAuth
		case upstreamErr.Status == http.StatusTooManyRequests:
			return telemetry.ErrRateLimit
		case upstreamErr.Status >= 500 && upstreamErr.Status <= 599:
			return telemetry.ErrServer
		case upstreamErr.Status >= 400 && upstreamErr.Status <= 499:
			return telemetry.ErrUpstream
		default:
			return telemetry.ErrConnection
		}
	}
	return telemetry.ErrConnection
}

func clientStatusForUpstream(proxyErr error, fallback int) int {
	var upstreamErr *provider.UpstreamError
	if errors.As(proxyErr, &upstreamErr) && upstreamErr.Status >= 400 && upstreamErr.Status <= 599 {
		return upstreamErr.Status
	}
	if fallback == http.StatusInternalServerError {
		return http.StatusBadGateway
	}
	return fallback
}

func (g *gatewayTelemetry) logFailure(action string, err error) {
	if g.logger != nil {
		g.logger.Warn("telemetry failure", "action", action, "error", err.Error())
	}
}
