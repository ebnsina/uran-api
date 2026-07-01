-- One GitHub OAuth connection per org: the user token we use to list repos.
CREATE TABLE IF NOT EXISTS github_connections (
	org_id        bigint PRIMARY KEY REFERENCES orgs (id) ON DELETE CASCADE,
	access_token  text        NOT NULL,
	account_login text        NOT NULL,
	created_at    timestamptz NOT NULL DEFAULT now()
);
