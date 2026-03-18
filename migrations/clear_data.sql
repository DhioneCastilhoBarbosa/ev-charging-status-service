-- Limpa todos os dados do banco (mantém tabelas e estrutura).
-- Ordem: tabelas filhas primeiro por causa das FKs.
TRUNCATE TABLE webhook_events, connector_status, stations, webhooks, third_party_credentials, users RESTART IDENTITY CASCADE;
