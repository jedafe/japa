//go:build integration

package testdb

import "testing"

// TestResolvedDatabaseName_Bypasses covers the two empirically-reproduced
// ways a hand-rolled net/url.Parse extraction disagreed with what pgx
// actually connects to, letting the primary "japa" database slip past the
// Connect guard undetected. Both must resolve to "japa" here so Connect's
// `name == primaryDatabaseName` check actually fires. This test needs no
// live database — it only exercises the pure DSN-parsing helper.
func TestResolvedDatabaseName_Bypasses(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"libpq keyword/value DSN": "host=localhost port=5432 dbname=japa user=japa sslmode=disable",
		"dbname query override":   "postgresql://japa:japa@localhost:5432/japa_test?sslmode=disable&dbname=japa",
	}

	for name, dsn := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := resolvedDatabaseName(dsn)
			if err != nil {
				t.Fatalf("resolvedDatabaseName(%q) returned unexpected error: %v", dsn, err)
			}

			if got != primaryDatabaseName {
				t.Fatalf("resolvedDatabaseName(%q) = %q, want %q — the guard would fail to fire on this DSN",
					dsn, got, primaryDatabaseName)
			}
		})
	}
}

// TestResolvedDatabaseName_SafeDefault confirms the default test DSN still
// resolves to the dedicated japa_test database, not the primary one.
func TestResolvedDatabaseName_SafeDefault(t *testing.T) {
	t.Parallel()

	got, err := resolvedDatabaseName(defaultTestDSN)
	if err != nil {
		t.Fatalf("resolvedDatabaseName(%q) returned unexpected error: %v", defaultTestDSN, err)
	}

	if got != "japa_test" {
		t.Fatalf("resolvedDatabaseName(%q) = %q, want %q", defaultTestDSN, got, "japa_test")
	}
}

// TestResolvedDatabaseName_FailsClosedOnGarbage proves an unparseable DSN
// returns a non-nil error rather than an empty/ambiguous name — Connect
// must treat a parse error as unsafe, never as "not the primary database".
func TestResolvedDatabaseName_FailsClosedOnGarbage(t *testing.T) {
	t.Parallel()

	_, err := resolvedDatabaseName("not a valid dsn at all ://???")
	if err == nil {
		t.Fatal("resolvedDatabaseName(garbage) returned a nil error; want a non-nil parse error so Connect fails closed")
	}
}
