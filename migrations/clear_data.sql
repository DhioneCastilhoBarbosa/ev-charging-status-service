-- Limpa todos os dados do banco (mantém tabelas e estrutura).
-- Ordem: tabelas filhas primeiro por causa das FKs.
TRUNCATE TABLE connector_status, stations, third_party_credentials, users RESTART IDENTITY CASCADE;
