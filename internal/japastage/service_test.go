package japastage

import (
	"strings"
	"testing"
)

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

func TestDirectoryRowToDTO_AvatarSizing(t *testing.T) {
	// Imgproxy config must be set before the first MediaURL call in this
	// test binary — loadKeySalt decodes env vars once via sync.Once.
	t.Setenv("JC_IMGPROXY_KEY", "736563726574") // "secret"
	t.Setenv("JC_IMGPROXY_SALT", "68656c6c6f")  // "hello"
	t.Setenv("JC_IMGPROXY_BASE_URL", "https://img.example.com")

	t.Run("media-backed avatar emits display-sized derivative", func(t *testing.T) {
		bucket, path, stored := "avatars", "u/42.webp", "https://img.example.com/old/sig/rs:auto:640:0/abc.webp"
		dto := directoryRowToDTO(DirectoryRow{
			AvatarURL:         &stored,
			AvatarMediaBucket: &bucket,
			AvatarMediaPath:   &path,
		})
		// Signed imgproxy URL when key/salt are configured, local serve
		// endpoint otherwise — either way the stored URL is replaced by a
		// media-derived one.
		if dto.AvatarURL == stored {
			t.Fatalf("expected media-derived url, got stored %s", dto.AvatarURL)
		}
		if !strings.Contains(dto.AvatarURL, "/rs:auto:128:0/") &&
			!strings.Contains(dto.AvatarURL, "/api/v1/media/serve/avatars/u/42.webp") {
			t.Fatalf("expected 128px derivative or serve fallback, got %s", dto.AvatarURL)
		}
	})

	t.Run("no media row falls back to stored url", func(t *testing.T) {
		stored := "https://cdn.example.com/avatar.jpg"
		dto := directoryRowToDTO(DirectoryRow{AvatarURL: &stored})
		if dto.AvatarURL != stored {
			t.Fatalf("expected stored url passthrough, got %s", dto.AvatarURL)
		}
	})
}
