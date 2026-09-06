package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/nawocci/pogu/internal/service"
)

const ResponsesUpstreamPath = "v1/responses"

func ProxyResponsesStream(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, collector *SSEUsageCollector) error {
	translated, err := TranslateChatToResponses(body, true)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	resp, err := proxyStreamHead(ctx, c, p, secret, path, translated, true, w)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flusher, _ := w.(http.Flusher)
	translator := NewResponsesToChatTranslator(upstreamModelName(translated))
	write := func(frames [][]byte) bool { return writeFrames(w, flusher, collector, frames) }
	if err := scanSSE(ctx, resp.Body, func(event, data string) bool {
		if strings.TrimSpace(data) == "[DONE]" {
			return write(translator.Finish())
		}
		return write(translator.FeedEvent(event, []byte(data)))
	}); err != nil {
		return err
	}
	if !write(translator.Finish()) {
		return context.Canceled
	}
	return nil
}

func ProxyResponsesUnary(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, tee io.Writer) error {
	translated, err := TranslateChatToResponses(body, false)
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
	out, err := TranslateResponsesToChat(raw)
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

func ProxyResponsesInletStream(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, collector *SSEUsageCollector) error {
	chatBody, err := TranslateAnthropicToChat(body, true)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	responsesBody, err := TranslateChatToResponses(chatBody, true)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	resp, err := proxyStreamHead(ctx, c, p, secret, path, responsesBody, true, w)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flusher, _ := w.(http.Flusher)
	toChat := NewResponsesToChatTranslator(upstreamModelName(responsesBody))
	toAnthropic := NewChatToAnthropicTranslator(upstreamModelName(responsesBody))
	write := func(frames [][]byte) bool { return writeFrames(w, flusher, collector, frames) }
	feedChat := func(payload []byte) bool {
		if string(payload) == "[DONE]" {
			return write(toAnthropic.Finish())
		}
		return write(toAnthropic.FeedChunk(payload))
	}
	if err := scanSSE(ctx, resp.Body, func(event, data string) bool {
		if data == "[DONE]" {
			return write(toAnthropic.Finish())
		}
		for _, frame := range toChat.FeedEvent(event, []byte(data)) {
			payload, ok := bytes.CutPrefix(bytes.TrimSpace(frame), []byte("data:"))
			if !ok {
				continue
			}
			if value := bytes.TrimSpace(payload); len(value) > 0 && !bytes.Equal(value, []byte("[DONE]")) {
				if !feedChat(value) {
					return false
				}
			}
		}
		return true
	}); err != nil {
		return err
	}
	if !write(toAnthropic.Finish()) {
		return context.Canceled
	}
	return nil
}

func ProxyResponsesInletUnary(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, tee io.Writer) error {
	chatBody, err := TranslateAnthropicToChat(body, false)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	responsesBody, err := TranslateChatToResponses(chatBody, false)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	req, err := c.NewRequest(ctx, p, secret, path, responsesBody, false)
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
	chatOut, err := TranslateResponsesToChat(raw)
	if err != nil {
		return &UpstreamError{Status: resp.StatusCode, Body: raw, Err: err}
	}
	out, err := TranslateChatToAnthropicObject(chatOut, upstreamModelName(responsesBody))
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
