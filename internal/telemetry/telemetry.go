package telemetry

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	StatusInProgress = "in_progress"
	StatusSuccess    = "success"
	StatusError      = "error"
)

type ErrorCategory string

const (
	ErrNone       ErrorCategory = ""
	ErrAuth       ErrorCategory = "auth_failure"
	ErrRateLimit  ErrorCategory = "rate_limit"
	ErrUpstream   ErrorCategory = "upstream_error"
	ErrServer     ErrorCategory = "upstream_failure"
	ErrConnection ErrorCategory = "connection_failure"
	ErrTimeout    ErrorCategory = "timeout"
	ErrCanceled   ErrorCategory = "canceled"
	ErrRouting    ErrorCategory = "routing_failure"
	ErrBadRequest ErrorCategory = "bad_request"
	ErrInternal   ErrorCategory = "internal_error"
)

type TokenUsage struct {
	Input  *int64
	Output *int64
	Total  *int64
}

func (u TokenUsage) Empty() bool { return u.Input == nil && u.Output == nil && u.Total == nil }

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
	Request
	Attempts []Attempt
}

type Recorder struct {
	db *sql.DB
}

func NewRecorder(db *sql.DB) *Recorder { return &Recorder{db: db} }

const writeTimeout = 5 * time.Second

func writeCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), writeTimeout)
}

func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate request id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func nullInt64(v *int64) any {
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

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func durationMS(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func (r *Recorder) StartRequest(req *Request, attempt *Attempt) error {
	ctx, cancel := writeCtx()
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin transaction: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO telemetry_requests(id,protocol,public_model,resolved_model,group_id,group_name,streaming,client_key_id,client_key_name,status,started_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		req.ID, req.Protocol, req.PublicModel, nullString(req.ResolvedModel), nullInt64(req.GroupID), nullString(req.GroupName),
		boolInt(req.Streaming), req.ClientKeyID, nullString(req.ClientKeyName), StatusInProgress,
		req.StartedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("telemetry: insert request: %w", err)
	}
	if err := insertAttempt(ctx, tx, attempt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Recorder) RecordResolution(id string, groupID *int64, groupName *string, resolvedModel string) error {
	ctx, cancel := writeCtx()
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE telemetry_requests SET resolved_model=?, group_id=?, group_name=? WHERE id=?`,
		resolvedModel, nullInt64(groupID), nullString(groupName), id)
	if err != nil {
		return fmt.Errorf("telemetry: record resolution: %w", err)
	}
	return nil
}

func (r *Recorder) StartAttempt(attempt *Attempt) error {
	ctx, cancel := writeCtx()
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := insertAttempt(ctx, tx, attempt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Recorder) FinishAttemptWithTiming(requestID string, number int, httpStatus *int, success bool, category ErrorCategory, usage TokenUsage, startedAt, completedAt time.Time) error {
	ctx, cancel := writeCtx()
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE telemetry_attempts SET http_status=?, success=?, error_category=?, input_tokens=?, output_tokens=?, total_tokens=?, completed_at=?, duration_ms=? WHERE request_id=? AND attempt_number=?`,
		nullInt(httpStatus), boolInt(success), category, nullInt64(usage.Input), nullInt64(usage.Output), nullInt64(usage.Total),
		completedAt.UTC().Format(time.RFC3339Nano), durationMS(startedAt, completedAt), requestID, number)
	if err != nil {
		return fmt.Errorf("telemetry: finish attempt: %w", err)
	}
	return nil
}

func (r *Recorder) FinishRequestWithTiming(id string, status string, httpStatus *int, category ErrorCategory, usage TokenUsage, ttftMS, upstreamMS *int64, servedModel *string, startedAt, completedAt time.Time) error {
	ctx, cancel := writeCtx()
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE telemetry_requests SET status=?, http_status=?, error_category=?, input_tokens=?, output_tokens=?, total_tokens=?, ttft_ms=?, upstream_ms=?, served_model=?, completed_at=?, duration_ms=? WHERE id=?`,
		status, nullInt(httpStatus), category, nullInt64(usage.Input), nullInt64(usage.Output), nullInt64(usage.Total),
		nullInt64(ttftMS), nullInt64(upstreamMS), nullString(servedModel),
		completedAt.UTC().Format(time.RFC3339Nano), durationMS(startedAt, completedAt), id)
	if err != nil {
		return fmt.Errorf("telemetry: finish request: %w", err)
	}
	return nil
}

func (r *Recorder) List(ctx context.Context, limit int) ([]RequestWithAttempts, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,protocol,public_model,resolved_model,served_model,group_id,group_name,streaming,client_key_id,client_key_name,status,http_status,error_category,started_at,completed_at,duration_ms,ttft_ms,upstream_ms,input_tokens,output_tokens,total_tokens
		 FROM telemetry_requests ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequestWithAttempts
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
		req.Streaming = streaming != 0
		if resolved.Valid {
			req.ResolvedModel = &resolved.String
		}
		if served.Valid {
			req.ServedModel = &served.String
		}
		if groupID.Valid {
			req.GroupID = &groupID.Int64
		}
		if groupName.Valid {
			req.GroupName = &groupName.String
		}
		if keyID.Valid {
			req.ClientKeyID = &keyID.Int64
		}
		if keyName.Valid {
			req.ClientKeyName = &keyName.String
		}
		if httpStatus.Valid {
			v := int(httpStatus.Int64)
			req.HTTPStatus = &v
		}
		req.ErrorCategory = ErrorCategory(category.String)
		req.StartedAt = parseTime(sql.NullString{String: started, Valid: true})
		if completed.Valid {
			t := parseTime(completed)
			req.CompletedAt = &t
		}
		if duration.Valid {
			req.DurationMS = &duration.Int64
		}
		if ttft.Valid {
			req.TTFTMS = &ttft.Int64
		}
		if upstream.Valid {
			req.UpstreamMS = &upstream.Int64
		}
		if in.Valid {
			req.Usage.Input = &in.Int64
		}
		if outTok.Valid {
			req.Usage.Output = &outTok.Int64
		}
		if total.Valid {
			req.Usage.Total = &total.Int64
		}
		out = append(out, RequestWithAttempts{Request: req})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()
	for i := range out {
		attempts, err := r.listAttempts(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Attempts = attempts
	}
	return out, nil
}

func (r *Recorder) listAttempts(ctx context.Context, requestID string) ([]Attempt, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT request_id,attempt_number,provider_id,provider_name,provider_prefix,provider_type,model_id,upstream_model,credential_id,started_at,completed_at,duration_ms,http_status,success,error_category,input_tokens,output_tokens,total_tokens
		 FROM telemetry_attempts WHERE request_id=? ORDER BY attempt_number`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attempt
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
		a.Success = success != 0
		if providerID.Valid {
			a.ProviderID = &providerID.Int64
		}
		if modelID.Valid {
			a.ModelID = &modelID.Int64
		}
		if credID.Valid {
			a.CredentialID = &credID.Int64
		}
		a.StartedAt = parseTime(started)
		if completed.Valid {
			t := parseTime(completed)
			a.CompletedAt = &t
		}
		if duration.Valid {
			a.DurationMS = &duration.Int64
		}
		if httpStatus.Valid {
			v := int(httpStatus.Int64)
			a.HTTPStatus = &v
		}
		a.ErrorCategory = ErrorCategory(category.String)
		if in.Valid {
			a.Usage.Input = &in.Int64
		}
		if outTok.Valid {
			a.Usage.Output = &outTok.Int64
		}
		if total.Valid {
			a.Usage.Total = &total.Int64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func insertAttempt(ctx context.Context, tx *sql.Tx, a *Attempt) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO telemetry_attempts(request_id,attempt_number,provider_id,provider_name,provider_prefix,provider_type,model_id,upstream_model,credential_id,started_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		a.RequestID, a.Number, a.ProviderID, a.ProviderName, a.ProviderPrefix, a.ProviderType, a.ModelID, a.UpstreamModel, a.CredentialID,
		a.StartedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("telemetry: insert attempt: %w", err)
	}
	return nil
}

func parseTime(v sql.NullString) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v.String)
	return t
}
