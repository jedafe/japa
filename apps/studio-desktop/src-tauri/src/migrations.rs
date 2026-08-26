use tauri_plugin_sql::{Migration, MigrationKind};

pub fn state_migrations() -> Vec<Migration> {
	vec![Migration {
		version: 1,
		description: "create_state_tables",
		sql: include_str!("migrations/001_state_tables.sql"),
		kind: MigrationKind::Up,
	}]
}
