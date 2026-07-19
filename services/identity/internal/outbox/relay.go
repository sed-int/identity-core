// Package outbox relays transactional outbox rows to Redis Streams (PRD §4.2).
//
// Publish-then-mark: a crash between XADD and the published_at update means
// the row is re-published on restart — delivery is at-least-once and
// consumers must be idempotent.
package outbox

import (
	"context"
	"database/sql"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	Stream       = "events:user"
	pollInterval = time.Second
	batchSize    = 100
)

type Relay struct {
	db  *sql.DB
	rdb *redis.Client
	log zerolog.Logger
}

func NewRelay(db *sql.DB, rdb *redis.Client, log zerolog.Logger) *Relay {
	return &Relay{db: db, rdb: rdb, log: log.With().Str("component", "outbox-relay").Logger()}
}

// Run polls until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) {
	r.log.Info().Str("stream", Stream).Msg("outbox relay started")
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.Info().Msg("outbox relay stopped")
			return
		case <-ticker.C:
			if err := r.publishBatch(ctx); err != nil && ctx.Err() == nil {
				r.log.Warn().Err(err).Msg("outbox publish batch failed; will retry")
			}
		}
	}
}

type row struct {
	id            int64
	aggregateType string
	aggregateID   string
	eventType     string
	payload       []byte
}

func (r *Relay) publishBatch(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload
		FROM outbox WHERE published_at IS NULL ORDER BY id LIMIT ?`, batchSize)
	if err != nil {
		return err
	}
	var batch []row
	for rows.Next() {
		var o row
		if err := rows.Scan(&o.id, &o.aggregateType, &o.aggregateID, &o.eventType, &o.payload); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, o := range batch {
		if err := r.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: Stream,
			Values: map[string]any{
				"outbox_id":      o.id,
				"aggregate_type": o.aggregateType,
				"aggregate_id":   o.aggregateID,
				"event_type":     o.eventType,
				"payload":        string(o.payload),
			},
		}).Err(); err != nil {
			return err
		}
		if _, err := r.db.ExecContext(ctx,
			`UPDATE outbox SET published_at = NOW(6) WHERE id = ?`, o.id); err != nil {
			return err
		}
		r.log.Info().Int64("outbox_id", o.id).Str("event_type", o.eventType).
			Str("aggregate_id", o.aggregateID).Msg("event published")
	}
	return nil
}
