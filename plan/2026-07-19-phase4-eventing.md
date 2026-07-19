# Plan: Phase 4 — Eventing (outbox relay + board consumer)

> Status: 🚧 In progress

Goal (PRD §4.2): `user.created` flows from the identity DB's transactional
outbox to the Board service via Redis Streams, at-least-once, idempotently.

## Branch: `feat/user-events`

### Identity: outbox relay (`internal/outbox`)
- Poll loop (1s): `SELECT … WHERE published_at IS NULL ORDER BY id LIMIT 100`,
  `XADD` each to stream `events:user` (fields: outbox_id, event_type,
  aggregate_id, payload), then set `published_at`.
- Publish-then-mark ⇒ a crash between the two republishes on restart —
  at-least-once by design; consumers must be idempotent.
- Started in `app.Run`, stops with the run context.

### Board: consumer + authors read model (`internal/consumer`)
- Why a read model: posts store only `author_id` (the token sub). Nicknames
  live in the identity domain — instead of calling identity at read time
  (coupling the PoC exists to avoid), board replicates them locally from
  `user.created` events.
- Migration `000003_create_authors`: `user_id VARCHAR(64) PK, nickname` —
  upsert is `INSERT … ON DUPLICATE KEY UPDATE` ⇒ naturally idempotent.
- Consumer group `board` on `events:user` (`XGROUP CREATE … MKSTREAM`);
  on startup drain own pending entries (id `0`) then block on `>`;
  `XACK` after successful upsert.
- Board config gains `REDIS_ADDR`.

### Contract: additive `author_nickname` field on `board.v1.Post`
- `ListPosts`/`CreatePost` LEFT JOIN authors — additive, non-breaking.

### Verification
- `scripts/m4_smoke.sh` against compose: signup → outbox `published_at` set →
  `XLEN events:user` > 0 → board `authors` row → `ListPosts` shows nickname.
- Backlog demo: outbox rows from earlier smokes publish on first relay start.
