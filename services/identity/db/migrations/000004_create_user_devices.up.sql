-- Security sub-domain (PRD §5): backs device-change detection (§4.2, Phase 6).
CREATE TABLE user_devices (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id            BIGINT UNSIGNED NOT NULL,
    device_fingerprint VARCHAR(128)    NOT NULL,
    device_name        VARCHAR(100)    NULL,
    last_seen_at       DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_devices_user_fingerprint (user_id, device_fingerprint),
    CONSTRAINT fk_devices_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE = InnoDB;
