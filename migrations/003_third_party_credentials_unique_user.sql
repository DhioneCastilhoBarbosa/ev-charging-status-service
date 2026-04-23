-- Uma linha de credencial por usuário (evita duplicatas no job/websocket).
-- Remove registros antigos, mantendo o mais recente por user_id.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY updated_at DESC NULLS LAST, created_at DESC NULLS LAST
           ) AS rn
    FROM third_party_credentials
)
DELETE FROM third_party_credentials
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

CREATE UNIQUE INDEX IF NOT EXISTS idx_third_party_credentials_user_id
    ON third_party_credentials (user_id);
