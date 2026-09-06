package telemetry

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"time"
)

const (
	StatusInProgress = "in_progress"
	StatusSuccess    = "success"
	StatusError      = "error"
)

type ErrorCategory string

const (
	ErrorNone            ErrorCategory = ""
	ErrorAuthFailure     ErrorCategory = "auth_failure"
	ErrorRateLimit       ErrorCategory = "rate_limit"
	ErrorUpstream        ErrorCategory = "upstream_error"
	ErrorUpstreamFailure ErrorCategory = "upstream_failure"
	ErrorConnection      ErrorCategory = "connection_failure"
	ErrorTimeout         ErrorCategory = "timeout"
	ErrorCanceled        ErrorCategory = "canceled"
	ErrorRouting         ErrorCategory = "routing_failure"
	ErrorBadRequest      ErrorCategory = "bad_request"
	ErrorInternal        ErrorCategory = "internal_error"
)

func CategoryForStatus(status int) ErrorCategory {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrorAuthFailure
	case http.StatusTooManyRequests:
		return ErrorRateLimit
	default:
		if status >= 500 {
			return ErrorUpstreamFailure
		}
		return ErrorUpstream
	}
}

type TokenUsage struct {
	Input  *int64 `json:"input_tokens"`
	Output *int64 `json:"output_tokens"`
	Total  *int64 `json:"total_tokens"`
}

func (u TokenUsage) Empty() bool {
	return u.Input == nil && u.Output == nil && u.Total == nil
}

type Request struct {
	ID            string
	Protocol      string
	PublicModel   string
	ResolvedModel *string
	ServedModel   *string
	GroupID       *int64
	GroupName     *string
	Streaming     bool
	ClientKeyID   *int64
	ClientKeyName *string
	Status        string
	HTTPStatus    *int
	ErrorCategory ErrorCategory
	StartedAt     time.Time
	CompletedAt   *time.Time
	DurationMS    *int64
	TTFTMS        *int64
	UpstreamMS    *int64
	Usage         TokenUsage
}

type Attempt struct {
	RequestID      string
	Number         int
	ProviderID     *int64
	ProviderName   string
	ProviderPrefix string
	ProviderType   string
	ModelID        *int64
	UpstreamModel  string
	CredentialID   *int64
	StartedAt      time.Time
	CompletedAt    *time.Time
	DurationMS     *int64
	HTTPStatus     *int
	Success        bool
	ErrorCategory  ErrorCategory
	Usage          TokenUsage
}

type RequestWithAttempts struct {
	Request  Request   `json:"request"`
	Attempts []Attempt `json:"attempts"`
}

type Recorder struct {
	db *sql.DB
}

func NewRecorder(db *sql.DB) *Recorder {
	return &Recorder{db: db}
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		now := time.Now().UnixNano()
		return hex.EncodeToString([]byte{
			byte(now >> 56), byte(now >> 48), byte(now >> 40), byte(now >> 32),
			byte(now >> 24), byte(now >> 16), byte(now >> 8), byte(now),
			byte(now >> 56), byte(now >> 48), byte(now >> 40), byte(now >> 32),
			byte(now >> 24), byte(now >> 16), byte(now >> 8), byte(now),
		})
	}
	return hex.EncodeToString(b[:])
}

func (r *Recorder) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func durationMS(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func (r *Recorder) StartRequest(req Request, first Attempt) {
	if r == nil || r.db == nil {
		return
	}
	ctx, cancel := r.ctx()
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO telemetry_requests(id, protocol, public_model, streaming, client_key_id, client_key_name,
			status, started_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.Protocol, req.PublicModel, boolInt(req.Streaming),
		nullInt64(req.ClientKeyID), nullString(req.ClientKeyName),
		StatusInProgress, req.StartedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return
	}
	if err := insertAttempt(ctx, tx, req.ID, first); err != nil {
		return
	}
	_ = tx.Commit()
}

func insertAttempt(ctx context.Context, tx *sql.Tx, requestID string, att Attempt) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO telemetry_attempts(request_id, attempt_number, provider_id, provider_name, provider_prefix,
			provider_type, model_id, upstream_model, credential_id, started_at, success)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		requestID, att.Number, nullInt64(att.ProviderID), att.ProviderName, att.ProviderPrefix,
		att.ProviderType, nullInt64(att.ModelID), att.UpstreamModel, nullInt64(att.CredentialID),
		att.StartedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (r *Recorder) RecordResolution(id string, resolved *string, groupID *int64, groupName *string) {
	if r == nil || r.db == nil {
		return
	}
	ctx, cancel := r.ctx()
	defer cancel()
	_, _ = r.db.ExecContext(ctx,
		`UPDATE telemetry_requests SET resolved_model=?, group_id=?, group_name=? WHERE id=?`,
		nullString(resolved), nullInt64(groupID), nullString(groupName), id)
}

func (r *Recorder) StartAttempt(att Attempt) {
	if r == nil || r.db == nil {
		return
	}
	ctx, cancel := r.ctx()
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if err := insertAttempt(ctx, tx, att.RequestID, att); err != nil {
		return
	}
	_ = tx.Commit()
}

func (r *Recorder) FinishAttemptWithTiming(requestID string, number int, status *int, success bool, category ErrorCategory, usage TokenUsage, started, completed time.Time) {
	if r == nil || r.db == nil {
		return
	}
	ctx, cancel := r.ctx()
	defer cancel()
	d := durationMS(started, completed)
	_, _ = r.db.ExecContext(ctx, `
		UPDATE telemetry_attempts SET completed_at=?, duration_ms=?, http_status=?, success=?,
			error_category=?, input_tokens=?, output_tokens=?, total_tokens=?
		WHERE request_id=? AND attempt_number=?`,
		completed.UTC().Format(time.RFC3339Nano), d, nullInt(status), boolInt(success),
		string(category), nullInt64(usage.Input), nullInt64(usage.Output), nullInt64(usage.Total),
		requestID, number)
}

func (r *Recorder) FinishRequestWithTiming(id string, status string, httpStatus *int, category ErrorCategory, usage TokenUsage, ttft, upstream *int64, served *string, started, completed time.Time) {
	if r == nil || r.db == nil {
		return
	}
	ctx, cancel := r.ctx()
	defer cancel()
	d := durationMS(started, completed)
	_, _ = r.db.ExecContext(ctx, `
		UPDATE telemetry_requests SET status=?, http_status=?, error_category=?,
			input_tokens=?, output_tokens=?, total_tokens=?,
			ttft_ms=?, upstream_ms=?, served_model=?, completed_at=?, duration_ms=?
		WHERE id=?`,
		status, nullInt(httpStatus), string(category),
		nullInt64(usage.Input), nullInt64(usage.Output), nullInt64(usage.Total),
		nullInt64(ttft), nullInt64(upstream), nullString(served),
		completed.UTC().Format(time.RFC3339Nano), d, id)
}

func (r *Recorder) FinishRequest(id string, status string, httpStatus *int, category ErrorCategory, usage TokenUsage) {
	now := time.Now().UTC()
	r.FinishRequestWithTiming(id, status, httpStatus, category, usage, nil, nil, nil, now.Add(-time.Nanosecond), now)
}

func (r *Recorder) List(limit int) ([]RequestWithAttempts, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("telemetry is unavailable")
	}
	if limit <= 0 {
		limit = 50
	}
	ctx, cancel := r.ctx()
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, protocol, public_model, resolved_model, served_model, group_id, group_name,
			streaming, client_key_id, client_key_name, status, http_status, error_category,
			started_at, completed_at, duration_ms, ttft_ms, upstream_ms,
			input_tokens, output_tokens, total_tokens
		FROM telemetry_requests ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RequestWithAttempts{}
	for rows.Next() {
		var req Request
		var resolved, served, groupName, keyName, completed, category sql.NullString
		var groupID, keyID sql.NullInt64
		var httpStatus sql.NullInt64
		var duration, ttft, upstream sql.NullInt64
		var in, outTok, total sql.NullInt64
		var started string
		var streaming int
		if err := rows.Scan(&req.ID, &req.Protocol, &req.PublicModel, &resolved, &served,
			&groupID, &groupName, &streaming, &keyID, &keyName, &req.Status, &httpStatus, &category,
			&started, &completed, &duration, &ttft, &upstream, &in, &outTok, &total); err != nil {
			return nil, err
		}
		req.Streaming = streaming == 1
		if resolved.Valid {
			v := resolved.String
			req.ResolvedModel = &v
		}
		if served.Valid {
			v := served.String
			req.ServedModel = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			req.GroupID = &v
		}
		if groupName.Valid {
			v := groupName.String
			req.GroupName = &v
		}
		if keyID.Valid {
			v := keyID.Int64
			req.ClientKeyID = &v
		}
		if keyName.Valid {
			v := keyName.String
			req.ClientKeyName = &v
		}
		if httpStatus.Valid {
			v := int(httpStatus.Int64)
			req.HTTPStatus = &v
		}
		req.ErrorCategory = ErrorCategory(category.String)
		if t, err := time.Parse(time.RFC3339Nano, started); err == nil {
			req.StartedAt = t
		}
		if completed.Valid {
			if t, err := time.Parse(time.RFC3339Nano, completed.String); err == nil {
				req.CompletedAt = &t
			}
		}
		if duration.Valid {
			v := duration.Int64
			req.DurationMS = &v
		}
		if ttft.Valid {
			v := ttft.Int64
			req.TTFTMS = &v
		}
		if upstream.Valid {
			v := upstream.Int64
			req.UpstreamMS = &v
		}
		if in.Valid {
			v := in.Int64
			req.Usage.Input = &v
		}
		if outTok.Valid {
			v := outTok.Int64
			req.Usage.Output = &v
		}
		if total.Valid {
			v := total.Int64
			req.Usage.Total = &v
		}
		attempts, err := r.listAttempts(ctx, req.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, RequestWithAttempts{Request: req, Attempts: attempts})
	}
	return out, rows.Err()
}

func (r *Recorder) listAttempts(ctx context.Context, requestID string) ([]Attempt, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT request_id, attempt_number, provider_id, provider_name, provider_prefix, provider_type,
			model_id, upstream_model, credential_id, started_at, completed_at, duration_ms,
			http_status, success, error_category, input_tokens, output_tokens, total_tokens
		FROM telemetry_attempts WHERE request_id=? ORDER BY attempt_number`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attempt{}
	for rows.Next() {
		var a Attempt
		var providerID, modelID, credID sql.NullInt64
		var started, completed, category sql.NullString
		var duration, httpStatus sql.NullInt64
		var in, outTok, total sql.NullInt64
		var success int
		if err := rows.Scan(&a.RequestID, &a.Number, &providerID, &a.ProviderName, &a.ProviderPrefix,
			&a.ProviderType, &modelID, &a.UpstreamModel, &credID, &started, &completed, &duration,
			&httpStatus, &success, &category, &in, &outTok, &total); err != nil {
			return nil, err
		}
		a.Success = success == 1
		if providerID.Valid {
			v := providerID.Int64
			a.ProviderID = &v
		}
		if modelID.Valid {
			v := modelID.Int64
			a.ModelID = &v
		}
		if credID.Valid {
			v := credID.Int64
			a.CredentialID = &v
		}
		if t, err := time.Parse(time.RFC3339Nano, started.String); err == nil {
			a.StartedAt = t
		}
		if completed.Valid {
			if t, err := time.Parse(time.RFC3339Nano, completed.String); err == nil {
				a.CompletedAt = &t
			}
		}
		if duration.Valid {
			v := duration.Int64
			a.DurationMS = &v
		}
		if httpStatus.Valid {
			v := int(httpStatus.Int64)
			a.HTTPStatus = &v
		}
		a.ErrorCategory = ErrorCategory(category.String)
		if in.Valid {
			v := in.Int64
			a.Usage.Input = &v
		}
		if outTok.Valid {
			v := outTok.Int64
			a.Usage.Output = &v
		}
		if total.Valid {
			v := total.Int64
			a.Usage.Total = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
