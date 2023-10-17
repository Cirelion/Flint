package fun

var DBSchemas = []string{`
CREATE TABLE IF NOT EXISTS fun_settings (
	guild_id        bigint PRIMARY KEY,
	p_role BIGINT NOT NULL DEFAULT 0,
	p_channel BIGINT NOT NULL DEFAULT 0,
	topics     TEXT NOT NULL,
	nsfw_topics         TEXT NOT NULL,
	dad_jokes         TEXT NOT NULL,
	wyrs         TEXT NOT NULL,
	nsfw_wyrs         TEXT NOT NULL
)`,
	`ALTER TABLE fun_settings ADD COLUMN IF NOT EXISTS p_role BIGINT NOT NULL DEFAULT 0;`,
	`ALTER TABLE fun_settings ADD COLUMN IF NOT EXISTS p_channel BIGINT NOT NULL DEFAULT 0;`,
}
