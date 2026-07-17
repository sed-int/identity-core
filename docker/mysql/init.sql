-- DB-per-service boundary (PRD §2): one container, separate databases, no cross-DB joins.
CREATE DATABASE IF NOT EXISTS identity CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE IF NOT EXISTS board CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
