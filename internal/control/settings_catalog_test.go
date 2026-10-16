package control

import (
	"strings"
	"testing"
)

// Deprecated reachability keys with no settingsCatalog entry — a stored
// system_settings row for these still reaches the DB-row loop in
// ListSystemSettings, so the predicate must not rely on catalog metadata.
var cataloglessDeprecatedKeys = []string{
	"service_packages.enabled",
	"service_coupons.enabled",
	"crs_calculator.enabled",
	"eca_guide.enabled",
	"pof_calculator.enabled",
	"sop_generator.enabled",
}

func TestIsDeprecatedSettingKey(t *testing.T) {
	for key := range deprecatedReachabilitySettings {
		if !isDeprecatedSettingKey(key) {
			t.Errorf("isDeprecatedSettingKey(%q) = false, want true", key)
		}
	}
}

func TestIsDeprecatedSettingKeyCatalogless(t *testing.T) {
	// Regression (fix round 1): deprecated keys without a catalog entry must
	// still be filtered — getSettingMeta can never mark them Deprecated.
	for _, key := range cataloglessDeprecatedKeys {
		if _, inCatalog := settingsCatalog[key]; inCatalog {
			t.Fatalf("test premise broken: %q gained a catalog entry", key)
		}

		if !isDeprecatedSettingKey(key) {
			t.Errorf("catalogless deprecated key %q not filtered", key)
		}
	}
}

func TestGetSettingMetaDeprecated(t *testing.T) {
	meta := getSettingMeta("video.enabled", "true")
	if !meta.Deprecated {
		t.Fatal("video.enabled should be marked Deprecated")
	}

	if !strings.HasPrefix(meta.Description, "DEPRECATED — managed via feature_flags/video.enabled") {
		t.Fatalf("description missing DEPRECATED prefix: %q", meta.Description)
	}

	// A key that is not a migrated reachability toggle stays untouched.
	sub := getSettingMeta("intel.discovery.enabled", "true")
	if sub.Deprecated {
		t.Fatal("intel.discovery.enabled is a sub-behavior switch, not deprecated")
	}

	if strings.HasPrefix(sub.Description, "DEPRECATED") {
		t.Fatalf("sub-setting got DEPRECATED prefix: %q", sub.Description)
	}
}
