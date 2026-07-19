// Package consumer ingests identity events from Redis Streams (PRD §4.2)
// into the board's local read models.
//
// Delivery is at-least-once (outbox relay may republish, unacked entries are
// redelivered), so every handler must be idempotent — the authors upsert is.
package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	stream = "events:user"
	group  = "board"
)

type Consumer struct {
	rdb  *redis.Client
	db   *sql.DB
	log  zerolog.Logger
	name string // consumer name within the group
}

func New(rdb *redis.Client, db *sql.DB, log zerolog.Logger) *Consumer {
	host, _ := os.Hostname()
	if host == "" {
		host = "board-consumer"
	}
	return &Consumer{
		rdb: rdb, db: db, name: host,
		log: log.With().Str("component", "event-consumer").Logger(),
	}
}

// Run consumes until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) {
	if err := c.rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil &&
		!strings.Contains(err.Error(), "BUSYGROUP") {
		c.log.Error().Err(err).Msg("create consumer group")
		return
	}
	c.log.Info().Str("stream", stream).Str("group", group).Str("consumer", c.name).
		Msg("event consumer started")

	// First drain this consumer's pending (delivered but unacked) entries from
	// a previous crash, then block on new ones.
	cursor := "0"
	for ctx.Err() == nil {
		res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: c.name,
			Streams:  []string{stream, cursor},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			cursor = ">"
			continue
		}
		if err != nil {
			if ctx.Err() == nil {
				c.log.Warn().Err(err).Msg("read group failed; retrying")
				time.Sleep(time.Second)
			}
			continue
		}

		empty := true
		for _, s := range res {
			for _, msg := range s.Messages {
				empty = false
				if err := c.handle(ctx, msg); err != nil {
					// Leave unacked: redelivered on next pending drain.
					c.log.Warn().Err(err).Str("msg_id", msg.ID).Msg("event handling failed")
					continue
				}
				c.rdb.XAck(ctx, stream, group, msg.ID)
			}
		}
		if empty && cursor == "0" {
			cursor = ">" // pending backlog drained; switch to new entries
		}
	}
	c.log.Info().Msg("event consumer stopped")
}

func (c *Consumer) handle(ctx context.Context, msg redis.XMessage) error {
	eventType, _ := msg.Values["event_type"].(string)
	payload, _ := msg.Values["payload"].(string)

	switch eventType {
	case "user.created":
		return c.upsertAuthor(ctx, payload)
	default:
		// Unknown events are acked and skipped — the stream may grow event
		// types this consumer doesn't care about.
		c.log.Info().Str("event_type", eventType).Str("msg_id", msg.ID).Msg("event skipped")
		return nil
	}
}

func (c *Consumer) upsertAuthor(ctx context.Context, payload string) error {
	var evt struct {
		UserID   json.Number `json:"user_id"`
		Nickname string      `json:"nickname"`
	}
	if err := json.Unmarshal([]byte(payload), &evt); err != nil {
		return err
	}
	if evt.UserID.String() == "" || evt.Nickname == "" {
		return errors.New("user.created payload missing user_id or nickname")
	}
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO authors (user_id, nickname) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE nickname = VALUES(nickname)`,
		evt.UserID.String(), evt.Nickname)
	if err == nil {
		c.log.Info().Str("user_id", evt.UserID.String()).Str("nickname", evt.Nickname).
			Msg("author read model updated")
	}
	return err
}
