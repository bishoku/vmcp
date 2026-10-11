-- Local demo only: Keycloak has a separate database and login.
CREATE USER keycloak WITH PASSWORD 'keycloak-demo';
CREATE DATABASE keycloak OWNER keycloak;
