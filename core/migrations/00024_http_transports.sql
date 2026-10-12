-- +goose Up
-- Retain migration version 24; upstream transports are HTTP only.
SELECT 1;
-- +goose Down
SELECT 1;
