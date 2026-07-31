-- Sessão WS/JWT: última atividade de aplicação (não inclui ping/pong).
-- Token sem expira por tempo fixo; idle e delete invalidam a sessão.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS ws_last_activity_at TIMESTAMPTZ;

COMMENT ON COLUMN users.ws_last_activity_at IS
    'Última atividade de aplicação da sessão WS/JWT; NULL = sessão inválida.';
