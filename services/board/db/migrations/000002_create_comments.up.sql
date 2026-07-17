-- Schema per PRD §3; the comments API itself is out of the PoC's initial scope.
CREATE TABLE comments (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    post_id    BIGINT UNSIGNED NOT NULL,
    author_id  VARCHAR(64)     NOT NULL,
    content    TEXT            NOT NULL,
    created_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY idx_comments_post (post_id),
    CONSTRAINT fk_comments_post FOREIGN KEY (post_id) REFERENCES posts (id)
) ENGINE = InnoDB;
