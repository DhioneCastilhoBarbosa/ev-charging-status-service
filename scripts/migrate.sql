
CREATE TABLE users (
id UUID PRIMARY KEY,
email TEXT,
api_user TEXT,
api_password TEXT,
api_key TEXT
);

CREATE TABLE stations (
id UUID PRIMARY KEY,
user_id UUID,
external_id TEXT
);
