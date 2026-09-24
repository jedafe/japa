package japastage

import "testing"

func TestValidIntent(t *testing.T) {
	valid := []string{"relocation", "study", "work", "family", "visit", "business"}
	for _, v := range valid {
		if !ValidIntent(v) {
			t.Errorf("expected %q to be a valid intent", v)
		}
	}

	for _, v := range []string{"", "tourist", "relocation ", "STUDY", "digital_nomad"} {
		if ValidIntent(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestIsRelocationIntent(t *testing.T) {
	for _, v := range []string{"relocation", "study", "work", "family"} {
		if !IsRelocationIntent(v) {
			t.Errorf("expected %q to be a relocation-class intent", v)
		}
	}

	for _, v := range []string{"visit", "business", ""} {
		if IsRelocationIntent(v) {
			t.Errorf("expected %q to be non-relocation", v)
		}
	}
}

func TestIntentsOptionList(t *testing.T) {
	svc := &Service{}
	opts := svc.Intents()

	if len(opts) != len(validIntents) {
		t.Fatalf("expected %d intent options, got %d", len(validIntents), len(opts))
	}

	for i, v := range validIntents {
		if opts[i].Value != v {
			t.Errorf("option %d: expected value %q, got %q", i, v, opts[i].Value)
		}
		if opts[i].Label == "" {
			t.Errorf("option %d (%q): missing label", i, v)
		}
	}
}

func TestValidStageUnchanged(t *testing.T) {
	for _, v := range validStages {
		if !ValidStage(v) {
			t.Errorf("expected %q to remain a valid stage", v)
		}
	}
}
