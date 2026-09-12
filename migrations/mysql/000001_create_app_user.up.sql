-- Least-privilege DB users (see docs/security/owasp-top10-mapping.md, A05 Security Misconfiguration).
-- Migrations run as a privileged "gearshare_migrator" user (root in local dev);
-- the application itself connects as "gearshare_app", which can only read/write
-- data, never alter schema.
CREATE USER IF NOT EXISTS 'gearshare_app'@'%' IDENTIFIED BY 'app_password';
GRANT SELECT, INSERT, UPDATE, DELETE ON gearshare.* TO 'gearshare_app'@'%';
FLUSH PRIVILEGES;
