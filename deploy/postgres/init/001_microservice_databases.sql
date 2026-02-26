-- Bootstrap isolated databases and least-privilege runtime accounts.
-- This script is executed by the postgres container on first initialization.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'gateway_app') THEN
    CREATE ROLE gateway_app LOGIN PASSWORD 'gateway_app_change_me' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'detection_app') THEN
    CREATE ROLE detection_app LOGIN PASSWORD 'detection_app_change_me' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'report_app') THEN
    CREATE ROLE report_app LOGIN PASSWORD 'report_app_change_me' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'governance_app') THEN
    CREATE ROLE governance_app LOGIN PASSWORD 'governance_app_change_me' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'gateway_db') THEN
    CREATE DATABASE gateway_db OWNER gateway_app;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'detection_db') THEN
    CREATE DATABASE detection_db OWNER detection_app;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'report_db') THEN
    CREATE DATABASE report_db OWNER report_app;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'governance_db') THEN
    CREATE DATABASE governance_db OWNER governance_app;
  END IF;
END $$;

REVOKE CONNECT ON DATABASE gateway_db FROM PUBLIC;
REVOKE CONNECT ON DATABASE detection_db FROM PUBLIC;
REVOKE CONNECT ON DATABASE report_db FROM PUBLIC;
REVOKE CONNECT ON DATABASE governance_db FROM PUBLIC;

GRANT CONNECT ON DATABASE gateway_db TO gateway_app;
GRANT CONNECT ON DATABASE detection_db TO detection_app;
GRANT CONNECT ON DATABASE report_db TO report_app;
GRANT CONNECT ON DATABASE governance_db TO governance_app;
