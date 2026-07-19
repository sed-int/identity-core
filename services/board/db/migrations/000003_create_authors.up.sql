-- Local read model of identity nicknames, replicated from `user.created`
-- events (PRD §4.2). The board NEVER queries the identity service for this.
CREATE TABLE authors (
    user_id    VARCHAR(64) NOT NULL,
    nickname   VARCHAR(50) NOT NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (user_id)
) ENGINE = InnoDB;
