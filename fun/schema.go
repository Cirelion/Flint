package fun

var DBSchemas = []string{`
CREATE TABLE IF NOT EXISTS fun_settings (
	guild_id        bigint PRIMARY KEY,
	topics     TEXT NOT NULL,
	nsfw_topics         TEXT NOT NULL,
	dad_jokes         TEXT NOT NULL,
	wyrs         TEXT NOT NULL,
	nsfw_wyrs         TEXT NOT NULL
)`,
}
