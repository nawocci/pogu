package telemetry

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	OutcomeSuccess   = "success"
	OutcomeFailed    = "failed"
	OutcomeCancelled = "cancelled"
)

type Range string

const (
	RangeDaily   Range = "daily"
	RangeMonthly Range = "monthly"
	RangeYearly  Range = "yearly"
)

func (r Range) Valid() bool {
	switch r {
	case RangeDaily, RangeMonthly, RangeYearly:
		return true
	}
	return false
}

func (r Range) window(now time.Time) (start, end, prevStart time.Time, bucketize func(time.Time) time.Time, interval string) {
	loc := now.Location()
	end = now
	y, m, d := now.Date()
	switch r {
	case RangeYearly:
		start = time.Date(y, time.January, 1, 0, 0, 0, 0, loc)
		prevStart = start.AddDate(-1, 0, 0)
		bucketize = func(t time.Time) time.Time {
			ty, tm, _ := t.Date()
			return time.Date(ty, tm, 1, 0, 0, 0, 0, loc)
		}
		interval = "month"
	case RangeMonthly:
		start = time.Date(y, m, 1, 0, 0, 0, 0, loc)
		prevStart = start.AddDate(0, -1, 0)
		bucketize = func(t time.Time) time.Time {
			ty, tm, td := t.Date()
			return time.Date(ty, tm, td, 0, 0, 0, 0, loc)
		}
		interval = "day"
	default:
		start = time.Date(y, m, d, 0, 0, 0, 0, loc)
		prevStart = start.AddDate(0, 0, -1)
		bucketize = func(t time.Time) time.Time {
			ty, tm, td := t.Date()
			th, _, _ := t.Clock()
			return time.Date(ty, tm, td, th, 0, 0, 0, loc)
		}
		interval = "hour"
	}
	return start, end, prevStart, bucketize, interval
}

func nextBucket(t time.Time, r Range) time.Time {
	switch r {
	case RangeYearly:
		return t.AddDate(0, 1, 0)
	case RangeMonthly:
		return t.AddDate(0, 0, 1)
	default:
		return t.Add(time.Hour)
	}
}

type Scope struct {
	KeyID    *int64
	Provider string
	Model    string
	Protocol string
	Group    string
	Stream   *bool
	Outcome  string
}

func (sc Scope) scopeClauses() (string, []any) {
	var conds []string
	var args []any
	if sc.KeyID != nil {
		conds = append(conds, `r.client_key_id = ?`)
		args = append(args, *sc.KeyID)
	}
	if sc.Provider != "" {
		conds = append(conds, `la.provider_name = ?`)
		args = append(args, sc.Provider)
	}
	if sc.Model != "" {
		conds = append(conds, `(r.public_model = ? OR COALESCE(r.resolved_model, '') = ? OR COALESCE(r.served_model, '') = ? OR EXISTS (SELECT 1 FROM telemetry_attempts a2 WHERE a2.request_id = r.id AND a2.upstream_model = ?))`)
		args = append(args, sc.Model, sc.Model, sc.Model, sc.Model)
	}
	if sc.Protocol != "" {
		conds = append(conds, `r.protocol = ?`)
		args = append(args, sc.Protocol)
	}
	if sc.Group != "" {
		conds = append(conds, `r.group_name = ? COLLATE NOCASE`)
		args = append(args, sc.Group)
	}
	if sc.Stream != nil {
		conds = append(conds, `r.streaming = ?`)
		if *sc.Stream {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	switch sc.Outcome {
	case OutcomeSuccess:
		conds = append(conds, `r.status = 'success'`)
	case OutcomeFailed:
		conds = append(conds, `r.status = 'error' AND COALESCE(r.error_category, '') != 'canceled'`)
	case OutcomeCancelled:
		conds = append(conds, `r.status = 'error' AND r.error_category = 'canceled'`)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(conds, " AND "), args
}

const monitorFrom = `FROM telemetry_requests r
	LEFT JOIN client_keys ck ON ck.id = r.client_key_id
	LEFT JOIN telemetry_attempts la ON la.request_id = r.id
		AND la.attempt_number = (SELECT MAX(attempt_number) FROM telemetry_attempts WHERE request_id = r.id)`

const monitorCols = `r.id,r.protocol,r.public_model,r.resolved_model,r.served_model,r.group_id,r.group_name,
	r.streaming,r.client_key_id,r.client_key_name,ck.name,
	r.status,r.http_status,r.error_category,r.started_at,r.completed_at,
	r.duration_ms,r.ttft_ms,r.upstream_ms,
	r.input_tokens,r.output_tokens,r.total_tokens,
	la.provider_name,la.provider_prefix,la.upstream_model`

type AttemptView struct {
	Number         int           `json:"number"`
	ProviderName   string        `json:"provider_name"`
	ProviderPrefix string        `json:"provider_prefix"`
	ProviderType   string        `json:"provider_type"`
	UpstreamModel  string        `json:"upstream_model"`
	CredentialID   *int64        `json:"credential_id"`
	HTTPStatus     *int          `json:"http_status"`
	Success        bool          `json:"success"`
	ErrorCategory  ErrorCategory `json:"error_category"`
	DurationMS     *int64        `json:"duration_ms"`
	InputTokens    *int64        `json:"input_tokens"`
	OutputTokens   *int64        `json:"output_tokens"`
	TotalTokens    *int64        `json:"total_tokens"`
}

type MonitoringRecord struct {
	RequestID     string        `json:"request_id"`
	Protocol      string        `json:"protocol"`
	TS            time.Time     `json:"ts"`
	KeyID         *int64        `json:"key_id"`
	KeyName       *string       `json:"key_name"`
	Model         string        `json:"model"`
	ResolvedModel *string       `json:"resolved_model"`
	ServedModel   *string       `json:"served_model"`
	GroupID       *int64        `json:"group_id"`
	GroupName     *string       `json:"group_name"`
	Provider      string        `json:"provider"`
	Status        *int          `json:"http_status"`
	Error         ErrorCategory `json:"error"`
	Cancelled     bool          `json:"cancelled"`
	Stream        bool          `json:"stream"`
	InputTokens   *int64        `json:"input_tokens"`
	OutputTokens  *int64        `json:"output_tokens"`
	TotalTokens   *int64        `json:"total_tokens"`
	LatencyMS     *int64        `json:"latency_ms"`
	TTFTMS        *int64        `json:"ttft_ms"`
	UpstreamMS    *int64        `json:"upstream_ms"`
	Attempts      []AttemptView `json:"attempts"`
}

type rowScan struct {
	rec          MonitoringRecord
	keyCurrent   sql.NullString
	lastProvider sql.NullString
}

func scanMonitoringRow(rows *sql.Rows) (rowScan, error) {
	var s rowScan
	var r *MonitoringRecord = &s.rec
	var streaming int
	var started, completed, resolved, served, groupName, keySnap sql.NullString
	var reqStatus sql.NullString
	var duration, httpStatus, groupID sql.NullInt64
	var category sql.NullString
	var lastPrefix, lastUpstream sql.NullString
	if err := rows.Scan(
		&r.RequestID, &r.Protocol, &r.Model, &resolved, &served, &groupID, &groupName,
		&streaming, &r.KeyID, &keySnap, &s.keyCurrent,
		&reqStatus, &httpStatus, &category, &started, &completed,
		&duration, &r.TTFTMS, &r.UpstreamMS,
		&r.InputTokens, &r.OutputTokens, &r.TotalTokens,
		&s.lastProvider, &lastPrefix, &lastUpstream,
	); err != nil {
		return s, err
	}
	_ = lastPrefix
	_ = lastUpstream
	if resolved.Valid {
		r.ResolvedModel = &resolved.String
	}
	if served.Valid {
		r.ServedModel = &served.String
	}
	if groupID.Valid {
		r.GroupID = &groupID.Int64
	}
	if groupName.Valid {
		r.GroupName = &groupName.String
	}
	if keySnap.Valid {
		r.KeyName = &keySnap.String
	} else if s.keyCurrent.Valid {
		r.KeyName = &s.keyCurrent.String
	}
	r.Stream = streaming != 0
	r.Error = ErrorCategory(category.String)
	r.TS = parseTime(started)
	if httpStatus.Valid {
		v := int(httpStatus.Int64)
		r.Status = &v
	} else {
		r.Status = nil
	}
	if duration.Valid {
		r.LatencyMS = &duration.Int64
	}
	r.Cancelled = r.Error == ErrCanceled
	if s.lastProvider.Valid {
		r.Provider = s.lastProvider.String
	}
	r.Attempts = []AttemptView{}
	return s, nil
}

func (r *Recorder) windowQuery(ctx context.Context, start, end time.Time, sc Scope, order string) (*sql.Rows, error) {
	extra, args := sc.scopeClauses()
	return r.db.QueryContext(ctx, rebuildWindowQuery(extra, order), combineArgs(start, end, args)...)
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func rebuildWindowQuery(extra, order string) string {
	return fmt.Sprintf(`SELECT %s %s
		WHERE r.started_at >= ? AND r.started_at < ? AND r.status != 'in_progress'%s
		ORDER BY %s`, monitorCols, monitorFrom, extra, order)
}

func combineArgs(start, end time.Time, scopeArgs []any) []any {
	out := []any{formatTime(start), formatTime(end)}
	return append(out, scopeArgs...)
}

func (r *Recorder) collectRows(ctx context.Context, rows *sql.Rows) ([]MonitoringRecord, error) {
	defer rows.Close()
	var scans []rowScan
	ids := map[string]int{}
	var order []string
	for rows.Next() {
		s, err := scanMonitoringRow(rows)
		if err != nil {
			return nil, err
		}
		if _, seen := ids[s.rec.RequestID]; !seen {
			ids[s.rec.RequestID] = len(scans)
			order = append(order, s.rec.RequestID)
			scans = append(scans, s)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(scans) == 0 {
		return []MonitoringRecord{}, nil
	}
	attByReq, err := r.attemptViewsFor(ctx, order)
	if err != nil {
		return nil, err
	}
	out := make([]MonitoringRecord, 0, len(scans))
	for _, s := range scans {
		rec := s.rec
		if atts, ok := attByReq[rec.RequestID]; ok {
			rec.Attempts = atts
		}
		out = append(out, rec)
	}
	return out, nil
}

func (r *Recorder) attemptViewsFor(ctx context.Context, ids []string) (map[string][]AttemptView, error) {
	out := make(map[string][]AttemptView, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	place := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
		out[id] = []AttemptView{}
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT request_id,attempt_number,provider_name,provider_prefix,provider_type,upstream_model,
			credential_id,http_status,success,error_category,duration_ms,input_tokens,output_tokens,total_tokens
		 FROM telemetry_attempts WHERE request_id IN (`+place+`) ORDER BY request_id, attempt_number`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var reqID string
		var a AttemptView
		var cred, httpStatus, duration sql.NullInt64
		var success int
		var category sql.NullString
		if err := rows.Scan(&reqID, &a.Number, &a.ProviderName, &a.ProviderPrefix, &a.ProviderType,
			&a.UpstreamModel, &cred, &httpStatus, &success, &category, &duration,
			&a.InputTokens, &a.OutputTokens, &a.TotalTokens); err != nil {
			return nil, err
		}
		if cred.Valid {
			a.CredentialID = &cred.Int64
		}
		if httpStatus.Valid {
			v := int(httpStatus.Int64)
			a.HTTPStatus = &v
		}
		if duration.Valid {
			a.DurationMS = &duration.Int64
		}
		a.Success = success != 0
		a.ErrorCategory = ErrorCategory(category.String)
		out[reqID] = append(out[reqID], a)
	}
	return out, rows.Err()
}

type tokenTotals struct {
	sum   int64
	known bool
}

func (t *tokenTotals) add(v *int64) {
	if v == nil {
		return
	}
	t.sum += *v
	t.known = true
}

func (t *tokenTotals) value() *int64 {
	if !t.known {
		return nil
	}
	v := t.sum
	return &v
}

type Totals struct {
	Requests     int64    `json:"requests"`
	Succeeded    int64    `json:"succeeded"`
	Failed       int64    `json:"failed"`
	Cancelled    int64    `json:"cancelled"`
	InputTokens  *int64   `json:"input_tokens"`
	OutputTokens *int64   `json:"output_tokens"`
	AvgLatencyMs *float64 `json:"avg_latency_ms"`
	SuccessRate  *float64 `json:"success_rate"`

	inputs  tokenTotals
	outputs tokenTotals
	latency struct {
		sum   int64
		count int64
	}
}

func (t *Totals) observe(rec *MonitoringRecord) {
	t.Requests++
	switch {
	case rec.Cancelled:
		t.Cancelled++
	case rec.Status != nil && *rec.Status >= 400 || rec.Error != "":
		t.Failed++
	default:
		t.Succeeded++
	}
	t.inputs.add(rec.InputTokens)
	t.outputs.add(rec.OutputTokens)
	if rec.LatencyMS != nil {
		t.latency.sum += *rec.LatencyMS
		t.latency.count++
	}
}

func (t *Totals) finish() {
	t.InputTokens = t.inputs.value()
	t.OutputTokens = t.outputs.value()
	if t.latency.count > 0 {
		avg := float64(t.latency.sum) / float64(t.latency.count)
		t.AvgLatencyMs = &avg
	}
	if eligible := t.Requests - t.Cancelled; eligible > 0 {
		rate := float64(t.Succeeded) / float64(eligible)
		t.SuccessRate = &rate
	}
}

type SummaryResult struct {
	Range    string    `json:"range"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Interval string    `json:"interval"`
	Current  Totals    `json:"current"`
	Previous Totals    `json:"previous"`
}

func (r *Recorder) observeWindow(ctx context.Context, start, end time.Time, sc Scope, t *Totals) error {
	rows, err := r.windowQuery(ctx, start, end, sc, "r.started_at, r.id")
	if err != nil {
		return err
	}
	recs, err := r.collectRows(ctx, rows)
	if err != nil {
		return err
	}
	for i := range recs {
		t.observe(&recs[i])
	}
	t.finish()
	return nil
}

func (r *Recorder) Summary(ctx context.Context, rng Range, now time.Time, sc Scope) (SummaryResult, error) {
	start, end, prevStart, _, interval := rng.window(now)
	res := SummaryResult{Range: string(rng), Start: start, End: end, Interval: interval}
	if err := r.observeWindow(ctx, start, end, sc, &res.Current); err != nil {
		return res, err
	}
	if err := r.observeWindow(ctx, prevStart, start, sc, &res.Previous); err != nil {
		return res, err
	}
	return res, nil
}

type Bucket struct {
	T            time.Time `json:"t"`
	Requests     int64     `json:"requests"`
	Succeeded    int64     `json:"succeeded"`
	Failed       int64     `json:"failed"`
	Cancelled    int64     `json:"cancelled"`
	InputTokens  *int64    `json:"input_tokens"`
	OutputTokens *int64    `json:"output_tokens"`
	AvgLatencyMs *float64  `json:"avg_latency_ms"`

	inputs  tokenTotals
	outputs tokenTotals
	latency struct {
		sum   int64
		count int64
	}
}

func (b *Bucket) observe(rec *MonitoringRecord) {
	b.Requests++
	switch {
	case rec.Cancelled:
		b.Cancelled++
	case rec.Status != nil && *rec.Status >= 400 || rec.Error != "":
		b.Failed++
	default:
		b.Succeeded++
	}
	b.inputs.add(rec.InputTokens)
	b.outputs.add(rec.OutputTokens)
	if rec.LatencyMS != nil {
		b.latency.sum += *rec.LatencyMS
		b.latency.count++
	}
}

func (b *Bucket) finish() {
	b.InputTokens = b.inputs.value()
	b.OutputTokens = b.outputs.value()
	if b.latency.count > 0 {
		avg := float64(b.latency.sum) / float64(b.latency.count)
		b.AvgLatencyMs = &avg
	}
}

func (r *Recorder) TimeSeries(ctx context.Context, rng Range, now time.Time, sc Scope) ([]Bucket, error) {
	start, end, _, bucketize, _ := rng.window(now)
	rows, err := r.windowQuery(ctx, start, end, sc, "r.started_at, r.id")
	if err != nil {
		return nil, err
	}
	recs, err := r.collectRows(ctx, rows)
	if err != nil {
		return nil, err
	}
	spans := map[time.Time]*Bucket{}
	for i := range recs {
		bt := bucketize(recs[i].TS.In(now.Location()))
		b, ok := spans[bt]
		if !ok {
			b = &Bucket{T: bt}
			spans[bt] = b
		}
		b.observe(&recs[i])
	}
	var out []Bucket
	for bt := bucketize(start); !bt.After(end); bt = nextBucket(bt, rng) {
		if b, ok := spans[bt]; ok {
			b.finish()
			out = append(out, *b)
		} else {
			out = append(out, Bucket{T: bt})
		}
	}
	if out == nil {
		out = []Bucket{}
	}
	return out, nil
}

type RequestPage struct {
	Items    []MonitoringRecord `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

func (r *Recorder) Requests(ctx context.Context, rng Range, now time.Time, sc Scope, page, pageSize int) (RequestPage, error) {
	start, end, _, _, _ := rng.window(now)
	res := RequestPage{Page: page, PageSize: pageSize, Items: []MonitoringRecord{}}
	extra, scopeArgs := sc.scopeClauses()
	countQ := fmt.Sprintf(`SELECT COUNT(*) %s
		WHERE r.started_at >= ? AND r.started_at < ? AND r.status != 'in_progress'%s`,
		monitorFrom, extra)
	if err := r.db.QueryRowContext(ctx, countQ, combineArgs(start, end, scopeArgs)...).Scan(&res.Total); err != nil {
		return res, err
	}
	if res.Total == 0 {
		return res, nil
	}
	selectQ := fmt.Sprintf(`SELECT %s %s
		WHERE r.started_at >= ? AND r.started_at < ? AND r.status != 'in_progress'%s
		ORDER BY r.started_at DESC, r.id DESC LIMIT ? OFFSET ?`,
		monitorCols, monitorFrom, extra)
	args := append(combineArgs(start, end, scopeArgs), pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, selectQ, args...)
	if err != nil {
		return res, err
	}
	items, err := r.collectRows(ctx, rows)
	if err != nil {
		return res, err
	}
	res.Items = items
	return res, nil
}

func (r *Recorder) GetRecord(ctx context.Context, id string) (*MonitoringRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+monitorCols+` `+monitorFrom+` WHERE r.id = ?`, id)
	if err != nil {
		return nil, err
	}
	recs, err := r.collectRows(ctx, rows)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, sql.ErrNoRows
	}
	return &recs[0], nil
}

func (r *Recorder) RecentRecords(ctx context.Context, n int) ([]MonitoringRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+monitorCols+` `+monitorFrom+`
		 WHERE r.status != 'in_progress' ORDER BY r.started_at DESC, r.id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	return r.collectRows(ctx, rows)
}

func (r *Recorder) ReapStale(now time.Time) error {
	ctx, cancel := writeCtx()
	defer cancel()
	ts := now.UTC().Format(time.RFC3339Nano)
	if _, err := r.db.ExecContext(ctx,
		`UPDATE telemetry_requests SET status='error', error_category='internal_error',
		 completed_at=?, duration_ms=NULL WHERE status='in_progress'`, ts); err != nil {
		return fmt.Errorf("telemetry: reap stale requests: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE telemetry_attempts SET success=0, error_category='internal_error',
		 completed_at=?, duration_ms=NULL
		 WHERE completed_at IS NULL AND request_id IN
		 (SELECT id FROM telemetry_requests WHERE status='error' AND error_category='internal_error' AND completed_at=?)`,
		ts, ts); err != nil {
		return fmt.Errorf("telemetry: reap stale attempts: %w", err)
	}
	return nil
}
