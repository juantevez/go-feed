-- Vincula cada refresh token con el access token emitido en el mismo par, para
-- que el logout (que solo recibe el access token) pueda revocar la sesión.
-- NULL en los tokens emitidos antes de esta migración.
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS access_jti VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_rt_access_jti ON refresh_tokens (access_jti);
