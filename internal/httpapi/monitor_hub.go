package httpapi

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/nawocci/pogu/internal/telemetry"
)

type monitorEvent struct {
	Event string
	Data  []byte
}

type monitorStarted struct {
	RequestID     string `json:"request_id"`
	TS            string `json:"ts"`
	KeyID         *int64 `json:"key_id,omitempty"`
	KeyName       string `json:"key_name,omitempty"`
	Model         string `json:"model"`
	Protocol      string `json:"protocol"`
	Stream        bool   `json:"stream"`
	Provider      string `json:"provider,omitempty"`
	UpstreamModel string `json:"upstream_model,omitempty"`
	ResolvedModel string `json:"resolved_model,omitempty"`
	GroupName     string `json:"group_name,omitempty"`
}

type monitorRouted struct {
	RequestID     string `json:"request_id"`
	Provider      string `json:"provider,omitempty"`
	UpstreamModel string `json:"upstream_model,omitempty"`
	ResolvedModel string `json:"resolved_model,omitempty"`
	GroupName     string `json:"group_name,omitempty"`
}

type monitorStreaming struct {
	RequestID   string `json:"request_id"`
	TTFTMS      int64  `json:"ttft_ms"`
	ServedModel string `json:"served_model,omitempty"`
}

type monitorHub struct {
	mu     sync.Mutex
	subs   map[chan monitorEvent]struct{}
	active map[string]*monitorStarted
}

const subBuffer = 256

func newMonitorHub() *monitorHub {
	return &monitorHub{
		subs:   make(map[chan monitorEvent]struct{}),
		active: make(map[string]*monitorStarted),
	}
}

func (h *monitorHub) subscribe() (chan monitorEvent, func(), []*monitorStarted) {
	ch := make(chan monitorEvent, subBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	active := make([]*monitorStarted, 0, len(h.active))
	for _, ev := range h.active {
		cp := *ev
		active = append(active, &cp)
	}
	h.mu.Unlock()
	return ch, func() { h.unsubscribe(ch) }, active
}

func (h *monitorHub) unsubscribe(ch chan monitorEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

func (h *monitorHub) publish(ev monitorEvent) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func marshalPayload(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func (h *monitorHub) publishStarted(ev *monitorStarted) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.active[ev.RequestID] = ev
	h.mu.Unlock()
	h.publish(monitorEvent{Event: "request_started", Data: marshalPayload(ev)})
}

func (h *monitorHub) publishRouted(ev *monitorRouted) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if active, ok := h.active[ev.RequestID]; ok {
		if ev.Provider != "" {
			active.Provider = ev.Provider
		}
		if ev.UpstreamModel != "" {
			active.UpstreamModel = ev.UpstreamModel
		}
		if ev.ResolvedModel != "" {
			active.ResolvedModel = ev.ResolvedModel
		}
		if ev.GroupName != "" {
			active.GroupName = ev.GroupName
		}
	}
	h.mu.Unlock()
	h.publish(monitorEvent{Event: "request_routed", Data: marshalPayload(ev)})
}

func (h *monitorHub) publishStreaming(ev *monitorStreaming) {
	if h == nil {
		return
	}
	h.publish(monitorEvent{Event: "request_streaming", Data: marshalPayload(ev)})
}

func (h *monitorHub) publishFinished(rec *telemetry.MonitoringRecord) {
	if h == nil || rec == nil {
		return
	}
	h.mu.Lock()
	delete(h.active, rec.RequestID)
	h.mu.Unlock()
	h.publish(monitorEvent{Event: "request_finished", Data: marshalPayload(rec)})
}

func nowString(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
