package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nawocci/pogu/internal/service"
)

type Client struct{ HTTP *http.Client }

func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second
	return &Client{HTTP: &http.Client{
		Transport: transport,
		Timeout:   0,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

var ErrUpstream = errors.New("upstream request failed")

type UpstreamError struct {
	Status int
	Body   []byte
	Err    error
}

func (e *UpstreamError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%v: %v", ErrUpstream, e.Err)
	}
	return fmt.Sprintf("%v: status %d", ErrUpstream, e.Status)
}

func (e *UpstreamError) Unwrap() error { return e.Err }

func endpointURL(p service.Provider, path string) (string, error) {
	base, err := url.Parse(p.BaseURL)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(base.Path, "/")
	route := strings.TrimLeft(path, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(route, "v1/") {
		route = strings.TrimPrefix(route, "v1/")
	}
	if basePath == "" {
		base.Path = "/" + route
	} else {
		base.Path = basePath + "/" + route
	}
	return base.String(), nil
}

func (c *Client) NewRequest(ctx context.Context, p service.Provider, secret, path string, body []byte, stream bool) (*http.Request, error) {
	endpoint, err := endpointURL(p, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.Type == service.ProviderAnthropic {
		req.Header.Set("x-api-key", secret)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	if isOpenCodeUpstream(p) {
		setOpenCodeHeaders(req)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	return req, nil
}

func isOpenCodeUpstream(p service.Provider) bool {
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "opencode.ai" || strings.HasSuffix(host, ".opencode.ai")
}

func setOpenCodeHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "opencode")
	req.Header.Set("x-opencode-client", "desktop")
	req.Header.Set("x-opencode-project", "global")
	req.Header.Set("x-opencode-session", "ses_"+randomHexID())
	req.Header.Set("x-opencode-request", "msg_"+randomHexID())
}

func randomHexID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		now := time.Now().UnixNano()
		return fmt.Sprintf("%016x%016x", now, now)
	}
	return hex.EncodeToString(b[:])
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c == nil || c.HTTP == nil {
		return nil, errors.New("provider HTTP client is unavailable")
	}
	return c.HTTP.Do(req)
}

func ReadError(resp *http.Response) *UpstreamError {
	if resp == nil {
		return &UpstreamError{Err: errors.New("empty upstream response")}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	return &UpstreamError{Status: resp.StatusCode, Body: body}
}

func (c *Client) Test(ctx context.Context, p service.Provider, secret string) error {
	endpoint, err := endpointURL(p, "/v1/models")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if p.Type == service.ProviderAnthropic {
		req.Header.Set("x-api-key", secret)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := c.Do(req)
	if err != nil {
		return &UpstreamError{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ReadError(resp)
	}
	_ = resp.Body.Close()
	return nil
}

func ProxyUnary(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, tee io.Writer) error {
	req, err := c.NewRequest(ctx, p, secret, path, body, false)
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
	copyHeaders(w, resp.Header)
	w.WriteHeader(resp.StatusCode)
	if tee != nil {
		_, err = io.Copy(w, io.TeeReader(resp.Body, tee))
		return err
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

func ProxyStream(ctx context.Context, c *Client, p service.Provider, secret, path string, body []byte, w http.ResponseWriter, collector *SSEUsageCollector) error {
	req, err := c.NewRequest(ctx, p, secret, path, body, true)
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
	copyHeaders(w, resp.Header)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/event-stream")
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	w.WriteHeader(resp.StatusCode)
	if flusher != nil {
		flusher.Flush()
	}
	reader := bufio.NewReader(resp.Body)
	var pending []byte
	flush := func(chunk []byte) bool {
		if _, err := w.Write(chunk); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
	tee := func(chunk []byte) bool {
		if collector != nil {
			data := append(pending, chunk...)
			pending = nil
			for {
				idx := bytes.IndexByte(data, '\n')
				if idx < 0 {
					break
				}
				line := bytes.TrimSuffix(data[:idx], []byte("\r"))
				if rest, ok := bytes.CutPrefix(line, []byte("data:")); ok {
					value := bytes.TrimSpace(rest)
					if len(value) > 0 && !bytes.Equal(value, []byte("[DONE]")) {
						collector.Observe(value)
					}
				}
				data = data[idx+1:]
			}
			pending = append(pending[:0], data...)
		}
		return flush(chunk)
	}
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := reader.Read(buf)
		if n > 0 {
			if !tee(buf[:n]) {
				return context.Canceled
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return &UpstreamError{Err: readErr}
		}
	}
}

func copyHeaders(dst http.ResponseWriter, src http.Header) {
	connectionTokens := make(map[string]struct{})
	for _, value := range src.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			connectionTokens[strings.ToLower(strings.TrimSpace(token))] = struct{}{}
		}
	}
	for key, values := range src {
		lower := strings.ToLower(key)
		if _, listed := connectionTokens[lower]; listed {
			continue
		}
		switch lower {
		case "connection", "content-length", "keep-alive", "proxy-authenticate", "proxy-authorization", "set-cookie", "te", "trailer", "transfer-encoding", "upgrade":
			continue
		}
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
}

func StatusMessage(err error) string {
	var upstream *UpstreamError
	if errors.As(err, &upstream) {
		var payload struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(upstream.Body, &payload) == nil && payload.Error.Message != "" {
			return payload.Error.Message
		}
		return fmt.Sprintf("upstream returned HTTP %d", upstream.Status)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "request canceled"
	}
	return "upstream unavailable"
}

func IsAuthFailure(err error) bool {
	var upstream *UpstreamError
	return errors.As(err, &upstream) && (upstream.Status == http.StatusUnauthorized || upstream.Status == http.StatusForbidden)
}

func HealthTimeout() time.Duration { return 10 * time.Second }
