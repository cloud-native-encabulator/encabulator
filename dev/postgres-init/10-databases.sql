-- Development databases for the Encabulator: one per owning service, as in design section 2.
-- Runs once, when the data directory is first initialised. Passwords are local constants;
-- the Helm chart generates real ones per install.
--
-- In PostgreSQL 15+ a non-owner cannot create objects in `public`, so each database's public
-- schema is handed to its owner as well.

CREATE ROLE hydrocoptic   LOGIN PASSWORD 'hydrocoptic';
CREATE ROLE detractor     LOGIN PASSWORD 'detractor';
CREATE ROLE girdlespring  LOGIN PASSWORD 'girdlespring';

CREATE DATABASE hydrocoptic  OWNER hydrocoptic;
CREATE DATABASE detractor    OWNER detractor;
CREATE DATABASE girdlespring OWNER girdlespring;

\connect hydrocoptic
ALTER SCHEMA public OWNER TO hydrocoptic;

\connect detractor
ALTER SCHEMA public OWNER TO detractor;

\connect girdlespring
ALTER SCHEMA public OWNER TO girdlespring;
