//go:build integration

package testdb

import (
	"testing"
)

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

// TestResolveDSN_IgnoresAppDatabaseURL is the regression test for the bug
// where resolveDSN fell back to JC_DATABASE_URL — the application's DSN,
// which legitimately points at the primary "japa" database in dev/prod.
// When JC_DATABASE_URL is set to the primary and JC_TEST_DATABASE_URL is
// unset, resolveDSN must return the dedicated japa_test default, NOT the
// primary DSN. Otherwise the Connect guard refuses every integration test
// in the repo (and, sans guard, the harness would seed/migrate real data).
//
// t.Setenv is incompatible with t.Parallel, so this test is serial by design
// (same convention as internal/config/config_test.go).
func TestResolveDSN_IgnoresAppDatabaseURL(t *testing.T) {
	//nolint:paralleltest // t.Setenv is incompatible with t.Parallel
	t.Setenv("JC_TEST_DATABASE_URL", "")
	t.Setenv("JC_DATABASE_URL", "postgresql://japa:japa@localhost:5432/japa?sslmode=disable")

	got := resolveDSN()
	if got != defaultTestDSN {
		t.Fatalf("resolveDSN() = %q, want %q — JC_DATABASE_URL must NOT be used as a fallback (it points at the primary database)",
			got, defaultTestDSN)
	}
}

// TestResolveDSN_PrefersTestDatabaseURL confirms an explicit JC_TEST_DATABASE_URL
// always wins over both the default and JC_DATABASE_URL.
func TestResolveDSN_PrefersTestDatabaseURL(t *testing.T) {
	//nolint:paralleltest // t.Setenv is incompatible with t.Parallel
	const want = "postgresql://custom:custom@db.example:5432/custom_test?sslmode=disable"
	t.Setenv("JC_TEST_DATABASE_URL", want)
	t.Setenv("JC_DATABASE_URL", "postgresql://japa:japa@localhost:5432/japa?sslmode=disable")

	if got := resolveDSN(); got != want {
		t.Fatalf("resolveDSN() = %q, want %q — JC_TEST_DATABASE_URL must take precedence", got, want)
	}
}

// TestResolveDSN_DefaultsToJapaTest confirms that with neither env var set,
// resolveDSN returns the dedicated japa_test default DSN.
func TestResolveDSN_DefaultsToJapaTest(t *testing.T) {
	//nolint:paralleltest // t.Setenv is incompatible with t.Parallel
	t.Setenv("JC_TEST_DATABASE_URL", "")
	t.Setenv("JC_DATABASE_URL", "")

	if got := resolveDSN(); got != defaultTestDSN {
		t.Fatalf("resolveDSN() = %q, want %q", got, defaultTestDSN)
	}
}
