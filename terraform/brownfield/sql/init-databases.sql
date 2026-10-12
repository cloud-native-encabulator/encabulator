-- Encabulator logical databases on RDS for PostgreSQL: one database and one owning role per
-- service (hydrocoptic, detractor, girdlespring), as dev/postgres-init/10-databases.sql
-- does for local development.
--
-- Run by the encabulator-db-init Job (kubernetes.tf) as the RDS master user, with
-- psql --set=ON_ERROR_STOP=1. Safe to run again: it creates only what is missing and
-- resets each role's password to the value in the Secret encabulator-db.
--
-- The RDS master user is not a superuser. PostgreSQL 16+ only lets it create a database
-- owned by a role it can SET ROLE to, hence the GRANT ... TO CURRENT_USER.

\getenv hydrocoptic_password HYDROCOPTIC_PASSWORD
\getenv detractor_password DETRACTOR_PASSWORD
\getenv girdlespring_password GIRDLESPRING_PASSWORD

-- Roles --------------------------------------------------------------------------------
SELECT format('CREATE ROLE %I LOGIN', r)
  FROM unnest(ARRAY['hydrocoptic', 'detractor', 'girdlespring']) AS r
 WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r)
\gexec

ALTER ROLE hydrocoptic  WITH LOGIN PASSWORD :'hydrocoptic_password';
ALTER ROLE detractor    WITH LOGIN PASSWORD :'detractor_password';
ALTER ROLE girdlespring WITH LOGIN PASSWORD :'girdlespring_password';

SELECT format('GRANT %I TO CURRENT_USER', r)
  FROM unnest(ARRAY['hydrocoptic', 'detractor', 'girdlespring']) AS r
 WHERE NOT pg_has_role(current_user, r, 'SET')
\gexec

-- Databases ----------------------------------------------------------------------------
SELECT format('CREATE DATABASE %I OWNER %I', d, d)
  FROM unnest(ARRAY['hydrocoptic', 'detractor', 'girdlespring']) AS d
 WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = d)
\gexec

-- In PostgreSQL 15+ only the database owner may create objects in `public`; hand each
-- database's public schema to its owner, as the local dev script does.
\connect hydrocoptic
ALTER SCHEMA public OWNER TO hydrocoptic;

\connect detractor
ALTER SCHEMA public OWNER TO detractor;

\connect girdlespring
ALTER SCHEMA public OWNER TO girdlespring;

\echo 'encabulator databases ready: hydrocoptic, detractor, girdlespring'
