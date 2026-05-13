-- Funcionalidade de webhook HTTP removida; fila e destinos não são mais usados.
DROP TABLE IF EXISTS webhook_events CASCADE;
DROP TABLE IF EXISTS webhooks CASCADE;
