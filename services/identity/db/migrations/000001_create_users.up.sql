-- Core master table (PRD §5): only identity + state, no volatile data.
CREATE TABLE users (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    status        ENUM ('PENDING', 'ACTIVE', 'DORMANT', 'SUSPENDED', 'DELETED')
                  NOT NULL DEFAULT 'PENDING',
    created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    last_login_at DATETIME(6) NULL,
    PRIMARY KEY (id)
) ENGINE = InnoDB;
