package ai

import "testing"

func TestParseAIPrefs(t *testing.T) {
	if p := ParseAIPrefs(nil); p.DefaultMode != "" || len(p.DisabledCategories) != 0 {
		t.Fatalf("nil prefs should be zero value, got %+v", p)
	}
	// Malformed JSON degrades to defaults rather than erroring.
	if p := ParseAIPrefs([]byte("{nope")); p.DefaultMode != "" {
		t.Fatalf("malformed prefs should degrade, got %+v", p)
	}
	p := ParseAIPrefs([]byte(`{"default_mode":"agent","disabled_categories":["forum"],"disabled_tools":["content.delete"],"enabled_tools":["content.search"]}`))
	if p.DefaultMode != "agent" || len(p.DisabledCategories) != 1 || len(p.DisabledTools) != 1 || len(p.EnabledTools) != 1 {
		t.Fatalf("prefs parse wrong: %+v", p)
	}
}

func TestFilterCatalog_PrefsAndOverride(t *testing.T) {
	cat := map[string]string{
		"content.search": "content", "content.delete": "content",
		"forum.threads.create": "forum", "crs.calculate": "calculators",
	}
	tools := []ToolConfig{
		{FunctionName: "content.search"}, {FunctionName: "content.delete"},
		{FunctionName: "forum.threads.create"}, {FunctionName: "crs.calculate"},
	}
	prefs := AIPrefs{
		DisabledCategories: []string{"forum"},
		DisabledTools:      []string{"content.delete"},
	}

	got := FilterCatalog(tools, cat, prefs, nil)
	if len(got) != 2 || got[0].FunctionName != "content.search" || got[1].FunctionName != "crs.calculate" {
		t.Fatalf("prefs filter wrong: %+v", got)
	}

	// Per-tool exception: re-enable one tool inside a disabled category.
	// Only "content" is disabled, so forum + calculators stay; result is
	// content.search + forum.threads.create + crs.calculate (content.delete
	// remains excluded).
	prefs2 := AIPrefs{DisabledCategories: []string{"content"}, EnabledTools: []string{"content.search"}}
	got = FilterCatalog(tools, cat, prefs2, nil)
	if len(got) != 3 {
		t.Fatalf("exception filter wrong: %+v", got)
	}
	for _, tc := range got {
		if tc.FunctionName == "content.delete" {
			t.Fatalf("disabled tool leaked: %+v", got)
		}
	}

	// Message override disables everything.
	off := false
	got = FilterCatalog(tools, cat, prefs, &ToolSelection{Enabled: &off})
	if len(got) != 0 {
		t.Fatalf("enabled:false should empty catalog, got %+v", got)
	}

	// Override whitelist narrows further.
	on := true
	got = FilterCatalog(tools, cat, AIPrefs{}, &ToolSelection{Enabled: &on, Categories: []string{"content"}})
	if len(got) != 2 {
		t.Fatalf("category override wrong: %+v", got)
	}
	for _, tc := range got {
		if cat[tc.FunctionName] != "content" {
			t.Fatalf("override leaked tool %s", tc.FunctionName)
		}
	}

	// Override tools whitelist.
	got = FilterCatalog(tools, cat, AIPrefs{}, &ToolSelection{Tools: []string{"crs.calculate"}})
	if len(got) != 1 || got[0].FunctionName != "crs.calculate" {
		t.Fatalf("tools whitelist wrong: %+v", got)
	}

	// Override cannot widen past prefs: enabled:true + categories:[forum]
	// with forum disabled by prefs → empty.
	got = FilterCatalog(tools, cat, prefs, &ToolSelection{Enabled: &on, Categories: []string{"forum"}})
	if len(got) != 0 {
		t.Fatalf("override must not widen past prefs, got %+v", got)
	}

	// An explicitly empty whitelist narrows to nothing — "tools":[] must not
	// fall back to the full catalog.
	got = FilterCatalog(tools, cat, AIPrefs{}, &ToolSelection{Enabled: &on, Tools: []string{}})
	if len(got) != 0 {
		t.Fatalf("empty tools whitelist should empty catalog, got %+v", got)
	}
	got = FilterCatalog(tools, cat, AIPrefs{}, &ToolSelection{Enabled: &on, Categories: []string{}})
	if len(got) != 0 {
		t.Fatalf("empty categories whitelist should empty catalog, got %+v", got)
	}
}
