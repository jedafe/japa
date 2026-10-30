// settings_test.go — unit coverage for §125 settings business logic: the
// PublicConfig projection, accepted-methods derivation, link validation,
// and the receive-eligibility seam (perm OR mentor). Store-backed paths are
// faked via tipStore (mentorship BookingBackend pattern).
package tips

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// fakeStore records calls and replays scripted settings/links state.
type fakeStore struct {
	settings      map[string]gen.TipSetting
	links         map[pgtype.UUID][]gen.TipExternalLink
	ocTips        map[string]gen.Tip
	inserted      []gen.InsertTipParams
	settled       []settleWalletParams
	emailToUser   map[string]pgtype.UUID
	idemTip       gen.Tip
	hasIdemTip    bool
	pointsToday   int64
	contentAuthor pgtype.UUID
	mentor        bool
	mentorErr     error
	orgOwner      pgtype.UUID
	getCalls      int
	upsertParams  gen.UpsertTipSettingsParams
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		settings:    map[string]gen.TipSetting{},
		links:       map[pgtype.UUID][]gen.TipExternalLink{},
		ocTips:      map[string]gen.Tip{},
		emailToUser: map[string]pgtype.UUID{},
	}
}

func key(t string, id pgtype.UUID) string { return t + ":" + id.String() }

func (f *fakeStore) getSettings(_ context.Context, t string, id pgtype.UUID) (gen.TipSetting, error) {
	f.getCalls++
	row, ok := f.settings[key(t, id)]
	if !ok {
		return gen.TipSetting{}, ErrNotFound
	}
	return row, nil
}

func (f *fakeStore) upsertSettings(_ context.Context, p gen.UpsertTipSettingsParams) (gen.TipSetting, error) {
	f.upsertParams = p
	row := gen.TipSetting{
		ID: p.OwnerID, OwnerType: p.OwnerType, OwnerID: p.OwnerID,
		TipsEnabled: p.TipsEnabled, AcceptPoints: p.AcceptPoints, AcceptCredits: p.AcceptCredits,
		AcceptFiat: p.AcceptFiat, AcceptExternal: p.AcceptExternal,
		MinTipAmount: p.MinTipAmount, ThankYouMessage: p.ThankYouMessage,
	}
	f.settings[key(p.OwnerType, p.OwnerID)] = row
	return row, nil
}

func (f *fakeStore) getSettingsByOCWebhookSecret(context.Context, string) (gen.TipSetting, error) {
	return gen.TipSetting{}, ErrNotFound
}
func (f *fakeStore) setOCCredentials(context.Context, gen.SetOCCredentialsParams) error { return nil }
func (f *fakeStore) clearOCCredentials(context.Context, string, pgtype.UUID) error      { return nil }
func (f *fakeStore) listSettingsWithOCToken(context.Context) ([]gen.TipSetting, error) {
	return nil, nil
}

func (f *fakeStore) createLink(_ context.Context, p gen.CreateTipExternalLinkParams) (gen.TipExternalLink, error) {
	row := gen.TipExternalLink{TipSettingsID: p.TipSettingsID, Kind: p.Kind, Label: p.Label, Url: p.Url, Position: p.Position}
	f.links[p.TipSettingsID] = append(f.links[p.TipSettingsID], row)
	return row, nil
}
func (f *fakeStore) updateLink(_ context.Context, p gen.UpdateTipExternalLinkParams) (gen.TipExternalLink, error) {
	return gen.TipExternalLink{ID: p.ID, Kind: p.Kind, Label: p.Label, Url: p.Url, Position: p.Position}, nil
}
func (f *fakeStore) deleteLink(context.Context, pgtype.UUID, pgtype.UUID) error { return nil }
func (f *fakeStore) listLinks(_ context.Context, id pgtype.UUID) ([]gen.TipExternalLink, error) {
	return f.links[id], nil
}

func (f *fakeStore) getTip(context.Context, pgtype.UUID) (gen.Tip, error) {
	return gen.Tip{}, ErrNotFound
}
func (f *fakeStore) getTipByIdempotencyKey(context.Context, pgtype.UUID, string) (gen.Tip, error) {
	if f.hasIdemTip {
		return f.idemTip, nil
	}
	return gen.Tip{}, ErrNotFound
}
func (f *fakeStore) getTipByPaymentID(context.Context, pgtype.UUID) (gen.Tip, error) {
	return gen.Tip{}, ErrNotFound
}
func (f *fakeStore) getTipByOCContributionID(_ context.Context, ocID string) (gen.Tip, error) {
	if t, ok := f.ocTips[ocID]; ok {
		return t, nil
	}
	return gen.Tip{}, ErrNotFound
}
func (f *fakeStore) insertTip(_ context.Context, p gen.InsertTipParams) (gen.Tip, error) {
	f.inserted = append(f.inserted, p)
	return gen.Tip{Method: p.Method, Status: p.Status}, nil
}
func (f *fakeStore) updateTipStatus(context.Context, pgtype.UUID, string) error { return nil }
func (f *fakeStore) settleTip(context.Context, pgtype.UUID, int64) error        { return nil }
func (f *fakeStore) attachTipPayment(context.Context, pgtype.UUID, pgtype.UUID, string) error {
	return nil
}

func (f *fakeStore) listTipsSent(context.Context, pgtype.UUID, int32, int32) ([]gen.Tip, error) {
	return nil, nil
}
func (f *fakeStore) listTipsReceived(context.Context, pgtype.UUID, int32, int32) ([]gen.Tip, error) {
	return nil, nil
}
func (f *fakeStore) countTipsReceived(context.Context, pgtype.UUID) (int32, error) { return 0, nil }
func (f *fakeStore) sumTipperPointsToday(context.Context, pgtype.UUID) (int64, error) {
	return f.pointsToday, nil
}
func (f *fakeStore) contentTipSummary(context.Context, pgtype.UUID) (gen.ContentTipSummaryRow, error) {
	return gen.ContentTipSummaryRow{}, nil
}
func (f *fakeStore) adminListTips(context.Context, gen.AdminListTipsParams) ([]gen.Tip, error) {
	return nil, nil
}
func (f *fakeStore) adminCountTips(context.Context, gen.AdminCountTipsParams) (int32, error) {
	return 0, nil
}
func (f *fakeStore) adminTipStats(context.Context, pgtype.Timestamptz) (gen.AdminTipStatsRow, error) {
	return gen.AdminTipStatsRow{}, nil
}
func (f *fakeStore) getOrgDisplayNames(context.Context, []pgtype.UUID) ([]gen.GetOrgDisplayNamesRow, error) {
	return nil, nil
}
func (f *fakeStore) getUserDisplayNames(context.Context, []pgtype.UUID) ([]gen.GetUserDisplayNamesRow, error) {
	return nil, nil
}
func (f *fakeStore) isMentor(context.Context, pgtype.UUID) (bool, error) {
	return f.mentor, f.mentorErr
}
func (f *fakeStore) getCurrencyByCode(context.Context, string) (gen.Currency, error) {
	return gen.Currency{}, ErrNotFound
}
func (f *fakeStore) getContentAuthorID(context.Context, pgtype.UUID) (pgtype.UUID, error) {
	if f.contentAuthor.Valid {
		return f.contentAuthor, nil
	}
	return pgtype.UUID{}, ErrNotFound
}
func (f *fakeStore) getUserIDByUsername(context.Context, string) (pgtype.UUID, error) {
	return pgtype.UUID{}, ErrNotFound
}
func (f *fakeStore) getUserEmail(context.Context, pgtype.UUID) (string, error) {
	return "sender@test.dev", nil
}
func (f *fakeStore) getUserIDByEmail(_ context.Context, email string) (pgtype.UUID, error) {
	if id, ok := f.emailToUser[email]; ok {
		return id, nil
	}
	return pgtype.UUID{}, ErrNotFound
}
func (f *fakeStore) getOrgOwnerID(context.Context, pgtype.UUID) (pgtype.UUID, error) {
	return f.orgOwner, nil
}
func (f *fakeStore) settleWalletTip(_ context.Context, p settleWalletParams) (gen.Tip, error) {
	if p.pointsDailyLimit > 0 && f.pointsToday+p.gross > p.pointsDailyLimit {
		return gen.Tip{}, ErrDailyPointsLimit
	}
	f.settled = append(f.settled, p)
	return gen.Tip{Method: p.method, Status: StatusCompleted}, nil
}
func (f *fakeStore) settleFiatTip(context.Context, pgtype.UUID, int64, pgtype.UUID) error {
	return nil
}
func (f *fakeStore) clawbackTip(context.Context, pgtype.UUID, pgtype.UUID) error { return nil }

type fakePerms struct{ grants map[string]bool }

func (p fakePerms) Can(_ context.Context, _ pgtype.UUID, node string) bool { return p.grants[node] }

func uid(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("bad uuid %q: %v", s, err)
	}
	return u
}

func TestGetSettingsDefaultWhenMissing(t *testing.T) {
	svc := &Service{store: newFakeStore()}
	st, err := svc.GetSettings(context.Background(), UserOwner(uid(t, "11111111-1111-1111-1111-111111111111")))
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if st.TipsEnabled || st.hasRow {
		t.Fatal("expected disabled synthesized settings")
	}
	if !st.AcceptCredits || !st.AcceptPoints || !st.AcceptFiat {
		t.Fatal("defaults should accept points/credits/fiat")
	}
}

func TestSettingsNeverExposeToken(t *testing.T) {
	fs := newFakeStore()
	owner := UserOwner(uid(t, "11111111-1111-1111-1111-111111111111"))
	token := "v1:key:ciphertext"
	slug := "mycollective"
	fs.settings[key(owner.Type, owner.ID)] = gen.TipSetting{
		OwnerType: owner.Type, OwnerID: owner.ID, TipsEnabled: true,
		OcCollectiveSlug: &slug, OcApiTokenEncrypted: &token,
	}
	svc := &Service{store: fs}
	st, err := svc.GetSettings(context.Background(), owner)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !st.HasOCToken {
		t.Fatal("HasOCToken should be derived from ciphertext presence")
	}
	if st.row.OcApiTokenEncrypted == nil {
		t.Fatal("row keeps ciphertext internally")
	}
}

func TestPublicConfigDisabled(t *testing.T) {
	svc := &Service{store: newFakeStore()}
	cfg, err := svc.PublicConfig(context.Background(), UserOwner(uid(t, "11111111-1111-1111-1111-111111111111")))
	if err != nil {
		t.Fatalf("PublicConfig: %v", err)
	}
	if cfg.Enabled || len(cfg.Methods) != 0 {
		t.Fatalf("disabled config should be empty, got %+v", cfg)
	}
}

func TestPublicConfigMethods(t *testing.T) {
	fs := newFakeStore()
	fs.mentor = true // recipient must be tips.receive-eligible for a public config
	owner := UserOwner(uid(t, "11111111-1111-1111-1111-111111111111"))
	slug := "collective"
	settingID := uid(t, "22222222-2222-2222-2222-222222222222")
	fs.settings[key(owner.Type, owner.ID)] = gen.TipSetting{
		ID: settingID, OwnerType: owner.Type, OwnerID: owner.ID, TipsEnabled: true,
		AcceptPoints: true, AcceptCredits: false, AcceptFiat: false, AcceptExternal: true,
		OcCollectiveSlug: &slug,
	}
	fs.links[settingID] = []gen.TipExternalLink{{Kind: "paypal", Label: "PayPal", Url: "https://paypal.me/x"}}

	svc := &Service{store: fs}
	cfg, err := svc.PublicConfig(context.Background(), owner)
	if err != nil {
		t.Fatalf("PublicConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
	want := map[string]bool{MethodPoints: true, MethodOpenCollective: true}
	for _, m := range cfg.Methods {
		delete(want, m)
	}
	if len(want) != 0 {
		t.Fatalf("missing methods: %v in %v", want, cfg.Methods)
	}
	if len(cfg.ExternalLinks) != 1 || cfg.ExternalLinks[0].Kind != "paypal" {
		t.Fatalf("links not projected: %+v", cfg.ExternalLinks)
	}
}

func TestCanReceivePermOrMentor(t *testing.T) {
	u := uid(t, "11111111-1111-1111-1111-111111111111")
	cases := []struct {
		name   string
		perms  map[string]bool
		mentor bool
		want   bool
	}{
		{"perm holder", map[string]bool{"tips.receive": true}, false, true},
		{"mentor bypass", nil, true, true},
		{"neither", nil, false, false},
	}
	for _, tc := range cases {
		fs := newFakeStore()
		fs.mentor = tc.mentor
		svc := &Service{store: fs, perms: fakePerms{tc.perms}}
		got, err := svc.CanReceive(context.Background(), u)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestUpdateSettingsForAuthorization(t *testing.T) {
	actor := uid(t, "11111111-1111-1111-1111-111111111111")
	other := uid(t, "22222222-2222-2222-2222-222222222222")

	svc := &Service{store: newFakeStore(), perms: fakePerms{map[string]bool{"tips.receive": true}}}
	if _, err := svc.UpdateSettingsFor(context.Background(), UserOwner(other), actor, UpdateSettingsInput{TipsEnabled: true}); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("expected ErrNotAuthorized for other user's row, got %v", err)
	}
	if _, err := svc.UpdateSettingsFor(context.Background(), UserOwner(actor), actor, UpdateSettingsInput{TipsEnabled: true}); err != nil {
		t.Fatalf("own row should update: %v", err)
	}

	svcNoPerm := &Service{store: newFakeStore(), perms: fakePerms{map[string]bool{}}}
	if _, err := svcNoPerm.UpdateSettingsFor(context.Background(), UserOwner(actor), actor, UpdateSettingsInput{TipsEnabled: true}); !errors.Is(err, ErrNotEligible) {
		t.Fatalf("expected ErrNotEligible, got %v", err)
	}
}

func TestLinkValidation(t *testing.T) {
	actor := uid(t, "11111111-1111-1111-1111-111111111111")
	fs := newFakeStore()
	svc := &Service{store: fs, perms: fakePerms{map[string]bool{"tips.receive": true}}}
	owner := UserOwner(actor)

	// First link auto-materializes the settings row — must fail on the
	// invalid-URL validation, not on a missing row.
	if _, err := svc.CreateLink(context.Background(), owner, actor, LinkInput{Kind: "paypal", Label: "x", URL: "notaurl"}); !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("expected ErrInvalidLink, got %v", err)
	}

	if _, err := svc.UpsertSettings(context.Background(), owner, UpdateSettingsInput{TipsEnabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	bad := []LinkInput{
		{Kind: "bogus", Label: "x", URL: "https://x.com"},
		{Kind: "paypal", Label: "", URL: "https://paypal.me/x"},
		{Kind: "paypal", Label: "x", URL: "ftp://paypal.me/x"},
		{Kind: "paypal", Label: "x", URL: "notaurl"},
	}
	for i, in := range bad {
		if _, err := svc.CreateLink(context.Background(), owner, actor, in); !errors.Is(err, ErrInvalidLink) {
			t.Fatalf("case %d: expected ErrInvalidLink, got %v", i, err)
		}
	}
	if _, err := svc.CreateLink(context.Background(), owner, actor, LinkInput{Kind: "paypal", Label: "PayPal", URL: "https://paypal.me/x"}); err != nil {
		t.Fatalf("valid link rejected: %v", err)
	}
}

func TestEnsureWritablePlatform(t *testing.T) {
	actor := uid(t, "11111111-1111-1111-1111-111111111111")
	svc := &Service{store: newFakeStore(), perms: fakePerms{map[string]bool{"tips.moderate": true}}}
	if err := svc.ensureWritable(context.Background(), PlatformOwner(), actor); err != nil {
		t.Fatalf("moderator should write platform row: %v", err)
	}
	svcDeny := &Service{store: newFakeStore(), perms: fakePerms{map[string]bool{}}}
	if err := svcDeny.ensureWritable(context.Background(), PlatformOwner(), actor); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("expected ErrNotAuthorized, got %v", err)
	}
}

func (f *fakeStore) failTip(context.Context, pgtype.UUID) error { return nil }
func (f *fakeStore) listStalePendingTips(context.Context, int32, int32) ([]pgtype.UUID, error) {
	return nil, nil
}
func (f *fakeStore) adminTipStatsByCurrency(context.Context, pgtype.Timestamptz) ([]gen.AdminTipStatsByCurrencyRow, error) {
	return nil, nil
}
func (f *fakeStore) sumRecipientPointsToday(context.Context, pgtype.UUID) (int64, error) {
	return 0, nil
}
func (f *fakeStore) getContentTitle(context.Context, pgtype.UUID) (string, error) {
	return "test content", nil
}
func (f *fakeStore) insertAuditLog(context.Context, pgtype.UUID, string, string, string, string) error {
	return nil
}
