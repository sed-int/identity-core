CREATE TABLE posts (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    -- The IdP's user id (token `sub` claim). No FK: the board DB never joins
    -- against identity data (PRD §2 DB-per-service).
    author_id  VARCHAR(64)     NOT NULL,
    title      VARCHAR(200)    NOT NULL,
    content    TEXT            NOT NULL,
    created_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY idx_posts_author (author_id)
) ENGINE = InnoDB;
