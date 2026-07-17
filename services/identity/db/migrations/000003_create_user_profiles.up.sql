-- Display sub-domain (PRD §5): frequently read/updated, separated from core state.
CREATE TABLE user_profiles (
    user_id           BIGINT UNSIGNED NOT NULL,
    nickname          VARCHAR(50)     NOT NULL,
    profile_image_url VARCHAR(512)    NULL,
    reputation_score  DOUBLE          NOT NULL DEFAULT 36.5,
    updated_at        DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (user_id),
    CONSTRAINT fk_profiles_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE = InnoDB;
