package bookmarkfolders

import "testing"

func TestResolveName(t *testing.T) {
	r := &DefaultFolderResolver{}

	tests := []struct {
		entityType  string
		contentType string
		want        string
	}{
		{"content", "blog_post", "blog"},
		{"content", "discussion", "discussions"},
		{"content", "guide", "guides"},
		{"content", "page", "pages"},
		{"content", "comparison", "comparisons"},
		{"content", "event", "events"},
		{"content", "job_listing", "jobs"},
		{"content", "story", "stories"},
		{"content", "achievement", "achievements"},
		{"content", "service_listing", "services"},
		{"content", "unknown_type", ""},
		{"blog_post", "", "blog"},
		{"academy_track", "", "learn"},
		{"doc_page", "", "docs"},
		{"forum_thread", "", "discussions"},
		{"forum_reply", "", "discussions"},
		{"thread", "", "discussions"},
		{"reply", "", "discussions"},
		{"space", "", "spaces"},
		{"tag", "", "tags"},
		{"comment", "", "comments"},
		{"unknown_entity", "", ""},
	}

	for _, tt := range tests {
		got := r.resolveName(tt.entityType, tt.contentType)
		if got != tt.want {
			t.Errorf("resolveName(%q, %q) = %q, want %q", tt.entityType, tt.contentType, got, tt.want)
		}
	}
}

func TestFolderPosition(t *testing.T) {
	if got, want := folderPosition("blog"), int32(0); got != want {
		t.Errorf("folderPosition(blog) = %d, want %d", got, want)
	}

	if got, want := folderPosition("discussions"), int32(1); got != want {
		t.Errorf("folderPosition(discussions) = %d, want %d", got, want)
	}

	if got, want := folderPosition("learn"), int32(10); got != want {
		t.Errorf("folderPosition(learn) = %d, want %d", got, want)
	}

	if got, want := folderPosition("unknown"), int32(0); got != want {
		t.Errorf("folderPosition(unknown) = %d, want %d", got, want)
	}
}
