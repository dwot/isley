package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// IdempotencyTTL is how long a stored response is replayed.
const IdempotencyTTL = 24 * time.Hour

// Error codes for idempotency keys.
const (
	CodeIdempotencyKeyReused     = "idempotency_key_reused"
	CodeIdempotencyKeyInProgress = "idempotency_key_in_progress"
)

// StoredResponse is the first response to an idempotent request.
type StoredResponse struct {
	Status   int
	Body     []byte
	Location string
}

// IdempotencyService remembers the responses to requests sent with an
// Idempotency-Key, per API key, for IdempotencyTTL.
type IdempotencyService struct {
	deps Deps
}

// Begin claims key for a request with the given hash. It returns:
//   - (nil, nil): the key is new; run the request, then Finish or Abandon.
//   - (resp, nil): the request already ran; replay resp.
//   - a Conflict: the key was used for a different request, or the first
//     request with it is still running.
func (s *IdempotencyService) Begin(ctx context.Context, apiKeyID int, key, requestHash string) (*StoredResponse, error) {
	now := s.deps.Clock.Now()
	if _, err := s.deps.DB.ExecContext(ctx, "DELETE FROM idempotency_key WHERE created_at < $1",
		s.deps.Zones.ToSystem(now.Add(-IdempotencyTTL))); err != nil {
		return nil, fmt.Errorf("expire idempotency keys: %w", err)
	}
	res, err := s.deps.DB.ExecContext(ctx, `
		INSERT INTO idempotency_key (api_key_id, idem_key, request_hash, created_at)
		VALUES ($1, $2, $3, $4) ON CONFLICT (api_key_id, idem_key) DO NOTHING`,
		apiKeyID, key, requestHash, s.deps.Zones.ToSystem(now))
	if err != nil {
		return nil, fmt.Errorf("claim idempotency key: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil, nil
	}
	var (
		hash     string
		status   int
		body     string
		location string
	)
	err = s.deps.DB.QueryRowContext(ctx,
		"SELECT request_hash, status, body, location FROM idempotency_key WHERE api_key_id = $1 AND idem_key = $2",
		apiKeyID, key).Scan(&hash, &status, &body, &location)
	if errors.Is(err, sql.ErrNoRows) {
		// Expired between the insert and the read; treat it as new.
		return s.Begin(ctx, apiKeyID, key, requestHash)
	}
	if err != nil {
		return nil, fmt.Errorf("read idempotency key: %w", err)
	}
	if hash != requestHash {
		return nil, Conflict(CodeIdempotencyKeyReused, "this Idempotency-Key was used for a different request")
	}
	if status == 0 {
		return nil, Conflict(CodeIdempotencyKeyInProgress, "a request with this Idempotency-Key is still running")
	}
	return &StoredResponse{Status: status, Body: []byte(body), Location: location}, nil
}

// Finish stores the response to replay for key.
func (s *IdempotencyService) Finish(ctx context.Context, apiKeyID int, key string, resp StoredResponse) error {
	_, err := s.deps.DB.ExecContext(ctx,
		"UPDATE idempotency_key SET status = $1, body = $2, location = $3 WHERE api_key_id = $4 AND idem_key = $5",
		resp.Status, string(resp.Body), resp.Location, apiKeyID, key)
	if err != nil {
		return fmt.Errorf("store idempotent response: %w", err)
	}
	return nil
}

// Abandon releases key after a request that failed, so a retry runs again.
func (s *IdempotencyService) Abandon(ctx context.Context, apiKeyID int, key string) error {
	_, err := s.deps.DB.ExecContext(ctx, "DELETE FROM idempotency_key WHERE api_key_id = $1 AND idem_key = $2", apiKeyID, key)
	if err != nil {
		return fmt.Errorf("release idempotency key: %w", err)
	}
	return nil
}
