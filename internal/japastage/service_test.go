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

func TestValidStageForIntent(t *testing.T) {
	shared := []string{"considering", "planning", "preparing", "applied", "approved"}

	for _, intent := range []string{"relocation", "study", "work", "family", ""} {
		for _, s := range append(append([]string{}, shared...), "relocated", "settled") {
			if !ValidStageForIntent(s, intent) {
				t.Errorf("expected %q valid for intent %q", s, intent)
			}
		}
		for _, s := range []string{"traveled", "returned", "bogus"} {
			if ValidStageForIntent(s, intent) {
				t.Errorf("expected %q rejected for intent %q", s, intent)
			}
		}
	}

	for _, intent := range []string{"visit", "business"} {
		for _, s := range append(append([]string{}, shared...), "traveled", "returned") {
			if !ValidStageForIntent(s, intent) {
				t.Errorf("expected %q valid for intent %q", s, intent)
			}
		}
		for _, s := range []string{"relocated", "settled", "bogus"} {
			if ValidStageForIntent(s, intent) {
				t.Errorf("expected %q rejected for intent %q", s, intent)
			}
		}
	}
}

func TestStagesFilteredByIntent(t *testing.T) {
	svc := &Service{}

	all := svc.Stages("")
	if len(all) != len(validStages) {
		t.Fatalf("expected %d stages unfiltered, got %d", len(validStages), len(all))
	}
	for _, o := range all {
		if len(o.Intents) == 0 {
			t.Errorf("stage %q missing intents metadata", o.Value)
		}
	}

	visit := svc.Stages("visit")
	if len(visit) != len(tripStages) {
		t.Fatalf("expected %d trip stages, got %d", len(tripStages), len(visit))
	}
	if visit[len(visit)-1].Value != "returned" {
		t.Errorf("expected last trip stage %q, got %q", "returned", visit[len(visit)-1].Value)
	}

	work := svc.Stages("work")
	if len(work) != len(relocationStages) {
		t.Fatalf("expected %d relocation stages, got %d", len(relocationStages), len(work))
	}
	if work[len(work)-1].Value != "settled" {
		t.Errorf("expected last relocation stage %q, got %q", "settled", work[len(work)-1].Value)
	}
}
