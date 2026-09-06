package httpapi

import (
	"net"
	"net/http"
	"time"
)

func (a *API) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" && r.URL.Path != "/api/auth/login" {
			if !a.validSession(r) {
				jsonError(w, http.StatusUnauthorized, "authentication required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return a.Service.ValidateSession(r.Context(), cookie.Value)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if !a.allowLogin(r) {
		jsonError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	select {
	case a.loginSlots <- struct{}{}:
		defer func() { <-a.loginSlots }()
	case <-r.Context().Done():
		return
	}
	ok, err := a.Service.CheckAdminPassword(r.Context(), in.Password)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "authentication unavailable")
		return
	}
	if !ok {
		a.recordLoginFailure(r)
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	a.clearLoginFailures(r)
	token, err := a.Service.CreateSession(r.Context(), 24*time.Hour)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "session unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 86400})
	jsonWrite(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = a.Service.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
	jsonWrite(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	jsonWrite(w, http.StatusOK, map[string]any{"authenticated": a.validSession(r)})
}

func (a *API) loginIdentity(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (a *API) allowLogin(r *http.Request) bool {
	identity := a.loginIdentity(r)
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	attempt := a.loginAttempts[identity]
	return attempt.BlockedUntil.IsZero() || time.Now().After(attempt.BlockedUntil)
}

func (a *API) recordLoginFailure(r *http.Request) {
	identity := a.loginIdentity(r)
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if len(a.loginAttempts) >= 256 {
		for id, at := range a.loginAttempts {
			if !at.BlockedUntil.IsZero() && time.Now().After(at.BlockedUntil) {
				delete(a.loginAttempts, id)
			}
		}
	}
	attempt := a.loginAttempts[identity]
	attempt.Count++
	if attempt.Count >= 5 {
		backoff := time.Duration(1<<uint(attempt.Count-5)) * time.Second
		if backoff > time.Minute {
			backoff = time.Minute
		}
		attempt.BlockedUntil = time.Now().Add(backoff)
	}
	if attempt.Count > 12 {
		attempt.Count = 12
	}
	a.loginAttempts[identity] = attempt
}

func (a *API) clearLoginFailures(r *http.Request) {
	a.loginMu.Lock()
	delete(a.loginAttempts, a.loginIdentity(r))
	a.loginMu.Unlock()
}
