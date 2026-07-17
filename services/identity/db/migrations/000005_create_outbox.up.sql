-- Transactional outbox (PRD §4.2): written in the same tx as domain changes,
-- relayed to Redis Streams by a background relay (Phase 4).
CREATE TABLE outbox (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    aggregate_type VARCHAR(50)     NOT NULL,
    aggregate_id   VARCHAR(64)     NOT NULL,
    event_type     VARCHAR(100)    NOT NULL,
    payload        JSON            NOT NULL,
    created_at     DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    published_at   DATETIME(6)     NULL,
    PRIMARY KEY (id),
    KEY idx_outbox_unpublished (published_at, id)
) ENGINE = InnoDB;
