SET SESSION cte_max_recursion_depth = 100000;
SET @base_id = 8000000000 + (CAST(@bench_run AS UNSIGNED) * 100000);

DELETE FROM user_devices WHERE user_id > @base_id AND user_id <= @base_id + @bench_count;
DELETE FROM user_credentials WHERE user_id > @base_id AND user_id <= @base_id + @bench_count;
DELETE FROM user_profiles WHERE user_id > @base_id AND user_id <= @base_id + @bench_count;
DELETE FROM users WHERE id > @base_id AND id <= @base_id + @bench_count;

INSERT INTO users (id, status)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < @bench_count
)
SELECT @base_id + n, 'ACTIVE' FROM seq;

INSERT INTO user_credentials (user_id, auth_type, identifier)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < @bench_count
)
SELECT @base_id + n, 'PHONE', CONCAT('+82', LPAD(@bench_run, 4, '0'), LPAD(n, 6, '0')) FROM seq;

INSERT INTO user_profiles (user_id, nickname)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < @bench_count
)
SELECT @base_id + n, CONCAT('phase7-login-', n) FROM seq;
