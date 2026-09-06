package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/nawocci/pogu/internal/service"
)

func openEventStream(w http.ResponseWriter, status int) http.Flusher {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	w.WriteHeader(status)
	if flusher != nil {
		flusher.Flush()
	}
	return flusher
}

func observeFrames(collector *SSEUsageCollector, frames [][]byte) {
	if collector == nil {
		return
	}
	for _, frame := range frames {
		for _, line := range bytes.Split(frame, []byte("\n")) {
			payload, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:"))
			if !ok {
				continue
			}
			if value := bytes.TrimSpace(payload); len(value) > 0 && !bytes.Equal(value, []byte("[DONE]")) {
				collector.Observe(value)
			}
		}
	}
}

func proxyStreamHead(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, stream bool, w http.ResponseWriter) (*http.Response, error) {
	req, err := c.NewRequest(ctx, p, secret, path, body, stream)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, &UpstreamError{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ReadError(resp)
	}
	openEventStream(w, resp.StatusCode)
	return resp, nil
}

func writeFrames(w http.ResponseWriter, flusher http.Flusher, collector *SSEUsageCollector, frames [][]byte) bool {
	for _, frame := range frames {
		if _, err := w.Write(frame); err != nil {
			return false
		}
	}
	observeFrames(collector, frames)
	if flusher != nil {
		flusher.Flush()
	}
	return true
}

func UpstreamPathForScheme(scheme string) string {
	switch scheme {
	case string(service.SchemeAnthropic):
		return AnthropicUpstreamPath
	default:
		return "v1/chat/completions"
	}
}

func ProxyAnthropicStream(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, collector *SSEUsageCollector) error {
	translated, err := TranslateChatToAnthropic(body, true)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	resp, err := proxyStreamHead(ctx, c, p, secret, path, translated, true, w)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flusher, _ := w.(http.Flusher)
	translator := NewAnthropicToChatTranslator(upstreamModelName(translated))
	write := func(frames [][]byte) bool { return writeFrames(w, flusher, collector, frames) }
	if err := scanSSE(ctx, resp.Body, func(event, data string) bool {
		return write(translator.FeedEvent(event, []byte(data)))
	}); err != nil {
		return err
	}
	if !write(translator.Finish()) {
		return context.Canceled
	}
	return nil
}

func ProxyAnthropicUnary(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, tee io.Writer) error {
	translated, err := TranslateChatToAnthropic(body, false)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	req, err := c.NewRequest(ctx, p, secret, path, translated, false)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ReadError(resp)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return &UpstreamError{Err: err}
	}
	out, err := TranslateAnthropicToChatObject(raw)
	if err != nil {
		return &UpstreamError{Status: resp.StatusCode, Body: raw, Err: err}
	}
	copyHeaders(w, resp.Header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if tee != nil {
		_, err = io.Copy(w, io.TeeReader(bytes.NewReader(out), tee))
		return err
	}
	_, err = w.Write(out)
	return err
}

func ProxyChatInletStream(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, collector *SSEUsageCollector) error {
	translated, err := TranslateAnthropicToChat(body, true)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	resp, err := proxyStreamHead(ctx, c, p, secret, path, translated, true, w)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flusher, _ := w.(http.Flusher)
	translator := NewChatToAnthropicTranslator(upstreamModelName(translated))
	write := func(frames [][]byte) bool { return writeFrames(w, flusher, collector, frames) }
	if err := scanSSE(ctx, resp.Body, func(_, data string) bool {
		if data == "[DONE]" {
			return write(translator.Finish())
		}
		return write(translator.FeedChunk([]byte(data)))
	}); err != nil {
		return err
	}
	if !write(translator.Finish()) {
		return context.Canceled
	}
	return nil
}

func ProxyChatInletUnary(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, tee io.Writer) error {
	translated, err := TranslateAnthropicToChat(body, false)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	req, err := c.NewRequest(ctx, p, secret, path, translated, false)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ReadError(resp)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return &UpstreamError{Err: err}
	}
	out, err := TranslateChatToAnthropicObject(raw, upstreamModelName(translated))
	if err != nil {
		return &UpstreamError{Status: resp.StatusCode, Body: raw, Err: err}
	}
	copyHeaders(w, resp.Header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if tee != nil {
		_, err = io.Copy(w, io.TeeReader(bytes.NewReader(out), tee))
		return err
	}
	_, err = w.Write(out)
	return err
}
