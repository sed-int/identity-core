-- Auth sub-domain (PRD §5): 1 user : N credentials.
-- password_hash is NULL for PHONE (OTP state lives in Redis, never here).
CREATE TABLE user_credentials (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id       BIGINT UNSIGNED NOT NULL,
    auth_type     ENUM ('PHONE', 'EMAIL') NOT NULL,
    identifier    VARCHAR(255)    NOT NULL,
    password_hash VARCHAR(255)    NULL,
    created_at    DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_credentials_type_identifier (auth_type, identifier),
    KEY idx_credentials_user_id (user_id),
    CONSTRAINT fk_credentials_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE = InnoDB;
