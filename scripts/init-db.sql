-- Runs once when the local Postgres volume is first initialized.
-- Integration tests create and drop their own databases; this base database
-- only needs to exist so the test helper can connect to "postgres".
CREATE DATABASE sela_test;
