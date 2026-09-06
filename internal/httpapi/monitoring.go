package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nawocci/pogu/internal/telemetry"
)

func parseMonitoringScope(r *http.Request) (telemetry.Scope, error) {
	q := r.URL.Query()
	var sc telemetry.Scope
	if v := q.Get("key_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return sc, errors.New("key_id must be a positive integer")
		}
		sc.KeyID = &id
	}
	sc.Provider = q.Get("provider")
	sc.Model = q.Get("model")
	if v := q.Get("protocol"); v != "" {
		if v != "openai" && v != "anthropic" {
			return sc, errors.New("protocol must be openai or anthropic")
		}
		sc.Protocol = v
	}
	sc.Group = q.Get("group")
	if v := q.Get("stream"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return sc, errors.New("stream must be true or false")
		}
		sc.Stream = &b
	}
	switch v := q.Get("status"); v {
	case "", telemetry.OutcomeSuccess, telemetry.OutcomeFailed, telemetry.OutcomeCancelled:
		sc.Outcome = v
	default:
		return sc, errors.New("status must be success, failed, or cancelled")
	}
	return sc, nil
}

func parseMonitoringRange(r *http.Request) (telemetry.Range, error) {
	rk := telemetry.Range(r.URL.Query().Get("range"))
	if rk == "" {
		rk = telemetry.RangeDaily
	}
	if !rk.Valid() {
		return rk, errors.New("range must be daily, monthly, or yearly")
	}
	return rk, nil
}

func (a *API) monitoringSummary(w http.ResponseWriter, r *http.Request) {
	rk, err := parseMonitoringRange(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	sc, err := parseMonitoringScope(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := a.Telemetry.Summary(r.Context(), rk, time.Now(), sc)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	jsonWrite(w, http.StatusOK, res)
}

func (a *API) monitoringTimeseries(w http.ResponseWriter, r *http.Request) {
	rk, err := parseMonitoringRange(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	sc, err := parseMonitoringScope(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	buckets, err := a.Telemetry.TimeSeries(r.Context(), rk, time.Now(), sc)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"buckets": buckets})
}

func (a *API) monitoringRequests(w http.ResponseWriter, r *http.Request) {
	rk, err := parseMonitoringRange(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	sc, err := parseMonitoringScope(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	res, err := a.Telemetry.Requests(r.Context(), rk, time.Now(), sc, page, pageSize)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	jsonWrite(w, http.StatusOK, res)
}

func (a *API) monitoringEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")

	msgs, unsub, active := a.Monitor.subscribe()
	defer unsub()

	recent, err := a.Telemetry.RecentRecords(r.Context(), 40)
	if err != nil {
		recent = []telemetry.MonitoringRecord{}
	}
	hello := struct {
		Active []*monitorStarted            `json:"active"`
		Recent []telemetry.MonitoringRecord `json:"recent"`
	}{Active: active, Recent: recent}
	if active == nil {
		hello.Active = []*monitorStarted{}
	}
	raw, err := json.Marshal(hello)
	if err != nil {
		return
	}
	if _, err := w.Write([]byte("event: hello\ndata: ")); err != nil {
		return
	}
	if _, err := w.Write(raw); err != nil {
		return
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return
	}
	flusher.Flush()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case msg, open := <-msgs:
			if !open {
				return
			}
			if _, err := io.WriteString(w, "event: "+msg.Event+"\ndata: "); err != nil {
				return
			}
			if _, err := w.Write(msg.Data); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
