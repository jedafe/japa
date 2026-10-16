package video

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChannelIDFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "channel path", url: "https://www.youtube.com/channel/UCu5J84CUcdoPBFy4LIZrBEQ", want: "UCu5J84CUcdoPBFy4LIZrBEQ"},
		{name: "channel path trailing", url: "https://www.youtube.com/channel/UCu5J84CUcdoPBFy4LIZrBEQ/videos", want: "UCu5J84CUcdoPBFy4LIZrBEQ"},
		{name: "feeds url", url: "https://www.youtube.com/feeds/videos.xml?channel_id=UCu5J84CUcdoPBFy4LIZrBEQ", want: "UCu5J84CUcdoPBFy4LIZrBEQ"},
		{name: "handle needs resolution", url: "https://www.youtube.com/@daadofficial", want: ""},
		{name: "c-name needs resolution", url: "https://www.youtube.com/c/DAAD", want: ""},
		{name: "non-youtube", url: "https://vimeo.com/channel/UCu5J84CUcdoPBFy4LIZrBEQ", want: ""},
		{name: "youtu.be needs resolution", url: "https://youtu.be/ABC123", want: ""},
		{name: "watch url", url: "https://www.youtube.com/watch?v=ABC123", want: ""},
		{name: "garbage", url: "not a url", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := channelIDFromURL(tt.url); got != tt.want {
				t.Fatalf("channelIDFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolveYouTubeChannelID(t *testing.T) {
	t.Parallel()

	t.Run("direct id skips fetch", func(t *testing.T) {
		t.Parallel()

		id, err := resolveYouTubeChannelID(t.Context(), http.DefaultClient,
			"https://www.youtube.com/channel/UCu5J84CUcdoPBFy4LIZrBEQ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if id != "UCu5J84CUcdoPBFy4LIZrBEQ" {
			t.Fatalf("got %q", id)
		}
	})

	t.Run("page scrape extracts channelId", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><script>var x = {"channelId":"UCu5J84CUcdoPBFy4LIZrBEQ"};</script></head></html>`))
		}))
		t.Cleanup(srv.Close)

		id, err := resolveYouTubeChannelID(t.Context(), srv.Client(), srv.URL+"/@daad")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if id != "UCu5J84CUcdoPBFy4LIZrBEQ" {
			t.Fatalf("got %q", id)
		}
	})

	t.Run("meta itemprop fallback", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><meta itemprop="identifier" content="UCu5J84CUcdoPBFy4LIZrBEQ"></head></html>`))
		}))
		t.Cleanup(srv.Close)

		id, err := resolveYouTubeChannelID(t.Context(), srv.Client(), srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if id != "UCu5J84CUcdoPBFy4LIZrBEQ" {
			t.Fatalf("got %q", id)
		}
	})

	t.Run("no id on page errors", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html><body>nothing here</body></html>`))
		}))
		t.Cleanup(srv.Close)

		if _, err := resolveYouTubeChannelID(t.Context(), srv.Client(), srv.URL); err == nil {
			t.Fatal("expected error when channel id absent")
		}
	})
}

func TestFetchChannelVideoIDs(t *testing.T) {
	t.Parallel()

	const feed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <yt:videoId>AAAAAAAAAAA</yt:videoId>
    <title>Video One</title>
  </entry>
  <entry>
    <yt:videoId>BBBBBBBBBBB</yt:videoId>
    <title>Video Two</title>
  </entry>
  <entry>
    <yt:videoId>AAAAAAAAAAA</yt:videoId>
    <title>Duplicate</title>
  </entry>
</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(feed))
	}))
	t.Cleanup(srv.Close)

	ids, err := fetchChannelVideoIDs(t.Context(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ids) != 2 || ids[0] != "AAAAAAAAAAA" || ids[1] != "BBBBBBBBBBB" {
		t.Fatalf("got %v", ids)
	}
}

func TestFetchChannelVideoIDsHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	if _, err := fetchChannelVideoIDs(t.Context(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected error on 404 feed")
	}
}
