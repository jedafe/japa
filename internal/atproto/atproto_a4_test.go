package atproto

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ipfs/go-cid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/multiformats/go-multihash"
)

// ── Facet byte-offsets (§127 A4's named risk: offsets index UTF-8 BYTES) ──

func TestComposePostText(t *testing.T) {
	url := "https://test.dev/t/slug-x"
	cases := []struct{ name, title, excerpt string }{
		{"plain", "hello", "world"},
		{"emoji", "🚀 launch day", "body text 🎉"},
		{"combining", "café", "naïve résumé"},
		{"multiscript", "のタイトル", "مرحبا بالعالم"},
		{"empty-title", "", "just a body"},
	}
	for _, c := range cases {
		text := composePostText(c.title, c.excerpt, url)
		if utf8.RuneCountInString(text) > postTextRunes {
			t.Fatalf("%s: %d runes > cap", c.name, utf8.RuneCountInString(text))
		}
		if !strings.HasSuffix(text, url) {
			t.Fatalf("%s: url not intact at tail: %q", c.name, text)
		}
		f := linkFacet(text, url)
		if f == nil {
			t.Fatalf("%s: no facet", c.name)
		}
		if got := text[f.Index.ByteStart:f.Index.ByteEnd]; got != url {
			t.Fatalf("%s: facet covers %q, want %q", c.name, got, url)
		}
		link := f.Features[0].RichtextFacet_Link
		if link == nil || link.Uri != url {
			t.Fatalf("%s: facet feature = %+v", c.name, f.Features[0])
		}
	}
}

func TestComposePostTextShrinksExcerptNotURL(t *testing.T) {
	// A title at its 120-rune cap plus an over-long body overflows 280 —
	// the excerpt must absorb the cut so the whole URL still anchors the tail.
	url := "https://test.dev/blog/" + strings.Repeat("s", 60)
	title := strings.Repeat("t", 120)
	body := strings.Repeat("b", 400)
	text := composePostText(title, body, url)
	if utf8.RuneCountInString(text) > postTextRunes {
		t.Fatalf("%d runes > cap", utf8.RuneCountInString(text))
	}
	if !strings.HasSuffix(text, url) {
		t.Fatalf("url truncated: tail %q", text[len(text)-20:])
	}
	f := linkFacet(text, url)
	if f == nil || text[f.Index.ByteStart:f.Index.ByteEnd] != url {
		t.Fatalf("facet = %+v", f)
	}
}

// ── Media → blob → embed (DB) ──

// fakeMediaReader serves canned object bytes keyed "bucket/path".
type fakeMediaReader struct{ objects map[string][]byte }

func (f fakeMediaReader) ReadObject(_ context.Context, bucket, path string, _ int64) ([]byte, error) {
	b, ok := f.objects[bucket+"/"+path]
	if !ok {
		return nil, fmt.Errorf("no object %s/%s", bucket, path)
	}
	return b, nil
}

// newA4Media inserts a media row for the user.
func newA4Media(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID, path, mime string, size int64, confirmed bool) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`INSERT INTO media (uploader_user_id, original_filename, mime_type, file_size_bytes,
		                    storage_path, storage_bucket, confirmed, alt_text, width_px, height_px)
		 VALUES ($1, 'img.jpg', $2, $3, $4, 'japaconnect', $5, 'alt txt', 800, 600) RETURNING id`,
		userID, mime, size, path, confirmed).Scan(&id)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM media WHERE id = $1`, id)
	})
	return id
}

func linkContentMedia(t *testing.T, ctx context.Context, d *db.DB, contentID, mediaID pgtype.UUID, fieldKey string, pos int) {
	t.Helper()
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO content_media_fields (content_id, media_id, field_key, position)
		 VALUES ($1, $2, $3, $4)`, contentID, mediaID, fieldKey, pos); err != nil {
		t.Fatalf("link media: %v", err)
	}
}

func TestDBFeedPostEmbed(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	img := []byte("fake-jpeg-bytes")
	svc.SetMediaReader(fakeMediaReader{objects: map[string][]byte{"japaconnect/a4/img1.jpg": img}})

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	mediaID := newA4Media(t, ctx, d, userID, "a4/img1.jpg", "image/jpeg", int64(len(img)), true)
	linkContentMedia(t, ctx, d, contentID, mediaID, "featured_image", 0)

	embed := svc.imageEmbed(ctx, acct.ID, contentID)
	if embed == nil || embed.EmbedImages == nil || len(embed.EmbedImages.Images) != 1 {
		t.Fatalf("embed = %+v", embed)
	}
	img0 := embed.EmbedImages.Images[0]
	if img0.Alt != "alt txt" {
		t.Fatalf("alt = %q", img0.Alt)
	}
	if img0.AspectRatio == nil || img0.AspectRatio.Width != 800 || img0.AspectRatio.Height != 600 {
		t.Fatalf("aspectRatio = %+v", img0.AspectRatio)
	}
	if img0.Image == nil || img0.Image.MimeType != "image/jpeg" || img0.Image.Size != int64(len(img)) {
		t.Fatalf("lexblob = %+v", img0.Image)
	}
	c, err := cid.Parse(img0.Image.Ref.String())
	if err != nil {
		t.Fatalf("blob ref not a cid: %v", err)
	}
	// The blob must be fetchable through the getBlob service path.
	mime, data, err := svc.BlobForDID(ctx, acct.Did, c.String())
	if err != nil || mime != "image/jpeg" || string(data) != string(img) {
		t.Fatalf("BlobForDID = %q %d bytes %v", mime, len(data), err)
	}

	// Full publish path — the record must carry the embed into the repo.
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}
	var blobCount int
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM atproto_blobs WHERE account_id = $1`, acct.ID).Scan(&blobCount); err != nil {
		t.Fatal(err)
	}
	if blobCount != 1 {
		t.Fatalf("blobs = %d, want 1", blobCount)
	}
}

func TestDBFeedPostEmbedSkips(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	svc.SetMediaReader(fakeMediaReader{objects: map[string][]byte{
		"japaconnect/a4/big.jpg":   make([]byte, embedImageMaxBytes+10),
		"japaconnect/a4/small.png": []byte("png"),
	}})

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")

	// Oversize, unconfirmed, and svg all skip; the small png embeds.
	big := newA4Media(t, ctx, d, userID, "a4/big.jpg", "image/jpeg", embedImageMaxBytes+10, true)
	unconf := newA4Media(t, ctx, d, userID, "a4/unconf.jpg", "image/jpeg", 10, false)
	svg := newA4Media(t, ctx, d, userID, "a4/logo.svg", "image/svg+xml", 10, true)
	small := newA4Media(t, ctx, d, userID, "a4/small.png", "image/png", 10, true)
	for i, m := range []pgtype.UUID{big, unconf, svg, small} {
		linkContentMedia(t, ctx, d, contentID, m, "item_media", i)
	}
	embed := svc.imageEmbed(ctx, acct.ID, contentID)
	if embed == nil || len(embed.EmbedImages.Images) != 1 {
		t.Fatalf("embed = %+v, want exactly the small png", embed)
	}
	if embed.EmbedImages.Images[0].Image.MimeType != "image/png" {
		t.Fatalf("embedded wrong image: %+v", embed.EmbedImages.Images[0].Image)
	}
}

// ── federate_opt_out (§127 A4 / §126) ──

func TestDBFederateOptOutGatesProjection(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := d.Writer.Exec(ctx,
		`UPDATE contents SET federate_opt_out = true WHERE id = $1`, contentID); err != nil {
		t.Fatal(err)
	}
	_, err := svc.PublishContent(ctx, contentID)
	if err == nil {
		t.Fatal("opted-out content published")
	}
	var p interface{ Permanent() bool }
	if !errors.As(err, &p) || !p.Permanent() {
		t.Fatalf("opt-out skip must be permanent, got %v", err)
	}
	var n int
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM atproto_records WHERE content_id = $1`, contentID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("record index row written for opted-out content")
	}
}

// ── Erasure purges blob bytes (§127 A4/GDPR) ──

func TestDBErasePurgesBlobs(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	blobCid, err := svc.PutBlob(ctx, acct.ID, "image/jpeg", []byte("x"))
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	if err := svc.EraseAccount(ctx, userID); err != nil {
		t.Fatalf("EraseAccount: %v", err)
	}
	if _, _, err := svc.GetBlob(ctx, acct.ID, blobCid); err == nil {
		t.Fatal("blob bytes survived erasure")
	}
	var n int
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM atproto_blobs WHERE account_id = $1`, acct.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d blob rows survived erasure", n)
	}
}

// ── Profile avatar/banner blobs (§127 A4) ──

func TestDBProfileAvatarBlob(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	avatar := []byte("avatar-bytes")
	svc.SetMediaReader(fakeMediaReader{objects: map[string][]byte{"japaconnect/a4/av.jpg": avatar}})

	mediaID := newA4Media(t, ctx, d, userID, "a4/av.jpg", "image/jpeg", int64(len(avatar)), true)
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO user_profiles (user_id, avatar_media_id) VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET avatar_media_id = $2`,
		userID, mediaID); err != nil {
		t.Fatalf("profile seed: %v", err)
	}

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	// The opt-in profile publish must have stored the avatar as a blob —
	// its CID serves back through getBlob.
	var blobCid string
	if err := d.Writer.QueryRow(ctx,
		`SELECT cid FROM atproto_blobs WHERE account_id = $1`, acct.ID).Scan(&blobCid); err != nil {
		t.Fatalf("avatar blob row: %v", err)
	}
	mime, data, err := svc.BlobForDID(ctx, acct.Did, blobCid)
	if err != nil || mime != "image/jpeg" || string(data) != string(avatar) {
		t.Fatalf("getBlob = %q %d %v", mime, len(data), err)
	}
}

// ── getBlob XRPC (§127 A4) ──

func TestGetBlobXRPC(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	h := NewHandlers(svc)

	blobCid, err := svc.PutBlob(ctx, acct.ID, "image/png", []byte("pngbytes"))
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}

	rec := httptest.NewRecorder()
	h.GetBlob(rec, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/xrpc/com.atproto.sync.getBlob?did=%s&cid=%s", acct.Did, blobCid), nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "pngbytes" ||
		rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("getBlob: %d %q %q", rec.Code, rec.Body, rec.Header().Get("Content-Type"))
	}

	// A CID that parses but was never stored → BlobNotFound lexicon name.
	sum, _ := multihash.Sum([]byte("never-stored"), multihash.SHA2_256, -1)
	absentCid := cid.NewCidV1(cid.Raw, sum).String()
	rec = httptest.NewRecorder()
	h.GetBlob(rec, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/xrpc/com.atproto.sync.getBlob?did=%s&cid=%s", acct.Did, absentCid), nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "BlobNotFound") {
		t.Fatalf("missing blob: %d %s", rec.Code, rec.Body)
	}

	// Malformed cid → InvalidRequest.
	rec = httptest.NewRecorder()
	h.GetBlob(rec, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/xrpc/com.atproto.sync.getBlob?did=%s&cid=not-a-cid", acct.Did), nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "InvalidRequest") {
		t.Fatalf("bad cid: %d %s", rec.Code, rec.Body)
	}

	// Disabled account → RepoDeactivated.
	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	rec = httptest.NewRecorder()
	h.GetBlob(rec, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/xrpc/com.atproto.sync.getBlob?did=%s&cid=%s", acct.Did, blobCid), nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "RepoDeactivated") {
		t.Fatalf("disabled: %d %s", rec.Code, rec.Body)
	}
}
