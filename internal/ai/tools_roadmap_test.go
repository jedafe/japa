package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Stub
// ---------------------------------------------------------------------------

type stubRoadmapService struct {
	page        *RoadmapItemPage
	item        *RoadmapItem
	categories  []RoadmapCategory
	listErr     error
	getErr      error
	createErr   error
	updateErr   error
	setStatErr  error
	listCatErr  error
	gotFilter   *RoadmapItemFilter
	gotCreate   *RoadmapItemInput
	gotUpdateID pgtype.UUID
	gotUpdate   *RoadmapItemInput
	gotStatusID pgtype.UUID
	gotStatus   string
	gotActorID  pgtype.UUID
}

func (s *stubRoadmapService) ListItems(ctx context.Context, f RoadmapItemFilter) (*RoadmapItemPage, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}

	s.gotFilter = &f
	if s.page != nil {
		return s.page, nil
	}

	return &RoadmapItemPage{Items: []RoadmapItem{}}, nil
}

func (s *stubRoadmapService) GetItem(ctx context.Context, id pgtype.UUID) (*RoadmapItem, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}

	return s.item, nil
}

func (s *stubRoadmapService) CreateItem(ctx context.Context, in RoadmapItemInput) (*RoadmapItem, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}

	s.gotCreate = &in
	if s.item != nil {
		return s.item, nil
	}

	return &RoadmapItem{Title: in.Title, Slug: in.Slug, Status: in.Status}, nil
}

func (s *stubRoadmapService) UpdateItem(ctx context.Context, id pgtype.UUID, in RoadmapItemInput) (*RoadmapItem, error) {
	if s.updateErr != nil {
		return nil, s.updateErr
	}

	s.gotUpdateID = id
	s.gotUpdate = &in

	return &RoadmapItem{ID: id.String(), Status: in.Status}, nil
}

func (s *stubRoadmapService) SetItemStatus(ctx context.Context, id pgtype.UUID, status string, actorID pgtype.UUID) (*RoadmapItem, error) {
	if s.setStatErr != nil {
		return nil, s.setStatErr
	}

	s.gotStatusID = id
	s.gotStatus = status
	s.gotActorID = actorID

	return &RoadmapItem{ID: id.String(), Status: status}, nil
}

func (s *stubRoadmapService) ListCategories(ctx context.Context) ([]RoadmapCategory, error) {
	if s.listCatErr != nil {
		return nil, s.listCatErr
	}

	return s.categories, nil
}

const testRoadmapUUID = "11111111-1111-1111-1111-111111111111"
const testRoadmapCatUUID = "22222222-2222-2222-2222-222222222222"

func roadmapDeps(svc *stubRoadmapService) *ToolDeps {
	return &ToolDeps{Roadmap: svc}
}

// decodeToolErr unwraps the {"error","code"} payload of an error ToolResult.
func decodeToolErr(t *testing.T, res *ToolResult) (code string) {
	t.Helper()

	if res == nil || res.Status != ToolCallStatusError {
		t.Fatalf("expected error result, got %+v", res)
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("unmarshal error content: %v", err)
	}

	return m["code"]
}

// ---------------------------------------------------------------------------
// categories.list
// ---------------------------------------------------------------------------

func TestRoadmapCategoriesList_NotConfigured(t *testing.T) {
	res, err := RoadmapCategoriesListHandler(&ToolDeps{})(context.Background(), nil, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "roadmap_unavailable" {
		t.Fatalf("expected roadmap_unavailable, got %s", got)
	}
}

func TestRoadmapCategoriesList_Success(t *testing.T) {
	svc := &stubRoadmapService{categories: []RoadmapCategory{
		{ID: testRoadmapCatUUID, Slug: "tools", Name: "Tools"},
	}}

	res, err := RoadmapCategoriesListHandler(roadmapDeps(svc))(context.Background(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("expected success, got %s: %s", res.Status, res.Content)
	}

	var out struct {
		Categories []RoadmapCategory `json:"categories"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Categories) != 1 || out.Categories[0].Slug != "tools" {
		t.Fatalf("unexpected categories: %+v", out.Categories)
	}
}

// ---------------------------------------------------------------------------
// items.list
// ---------------------------------------------------------------------------

func TestRoadmapItemsList_FiltersAndPaging(t *testing.T) {
	svc := &stubRoadmapService{page: &RoadmapItemPage{
		Items:      []RoadmapItem{{ID: testRoadmapUUID, Status: "shipped"}},
		Total:      42,
		Page:       2,
		TotalPages: 3,
	}}

	args := json.RawMessage(`{"status":"shipped","q":"spaces","category_id":"` + testRoadmapCatUUID + `","sort":"votes","page":2,"limit":20}`)
	res, err := RoadmapItemsListHandler(roadmapDeps(svc))(context.Background(), args, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("expected success, got %s: %s", res.Status, res.Content)
	}

	f := svc.gotFilter
	if f == nil {
		t.Fatal("service did not receive filter")
	}
	if f.Status != "shipped" || f.Search != "spaces" || f.Sort != "votes" {
		t.Fatalf("unexpected filter: %+v", f)
	}
	if f.CategoryID.String() != testRoadmapCatUUID {
		t.Fatalf("expected category %s, got %s", testRoadmapCatUUID, f.CategoryID)
	}
	if f.PageSize != 20 || f.PageOffset != 20 {
		t.Fatalf("expected size 20 offset 20, got %d/%d", f.PageSize, f.PageOffset)
	}
}

func TestRoadmapItemsList_DefaultsAndCap(t *testing.T) {
	svc := &stubRoadmapService{}

	if _, err := RoadmapItemsListHandler(roadmapDeps(svc))(context.Background(), json.RawMessage(`{"limit":500}`), pgtype.UUID{}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if svc.gotFilter.PageSize != roadmapListLimitCap {
		t.Fatalf("expected cap %d, got %d", roadmapListLimitCap, svc.gotFilter.PageSize)
	}
}

func TestRoadmapItemsList_InvalidCategoryID(t *testing.T) {
	res, err := RoadmapItemsListHandler(roadmapDeps(&stubRoadmapService{}))(context.Background(), json.RawMessage(`{"category_id":"nope"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "invalid_category_id" {
		t.Fatalf("expected invalid_category_id, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// item.get
// ---------------------------------------------------------------------------

func TestRoadmapItemGet_MissingAndInvalidID(t *testing.T) {
	h := RoadmapItemGetHandler(roadmapDeps(&stubRoadmapService{}))

	res, _ := h(context.Background(), json.RawMessage(`{}`), pgtype.UUID{})
	if got := decodeToolErr(t, res); got != "missing_id" {
		t.Fatalf("expected missing_id, got %s", got)
	}

	res, _ = h(context.Background(), json.RawMessage(`{"id":"nope"}`), pgtype.UUID{})
	if got := decodeToolErr(t, res); got != "invalid_id" {
		t.Fatalf("expected invalid_id, got %s", got)
	}
}

func TestRoadmapItemGet_NotFound(t *testing.T) {
	svc := &stubRoadmapService{getErr: &RoadmapServiceError{Code: "item_not_found", Message: "roadmap item not found"}}

	res, err := RoadmapItemGetHandler(roadmapDeps(svc))(context.Background(), json.RawMessage(`{"id":"`+testRoadmapUUID+`"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "item_not_found" {
		t.Fatalf("expected item_not_found, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// item.create
// ---------------------------------------------------------------------------

func TestRoadmapItemCreate_MissingTitle(t *testing.T) {
	res, err := RoadmapItemCreateHandler(roadmapDeps(&stubRoadmapService{}))(context.Background(), json.RawMessage(`{"slug":"x"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "missing_title" {
		t.Fatalf("expected missing_title, got %s", got)
	}
}

func TestRoadmapItemCreate_ShippedPassesActor(t *testing.T) {
	svc := &stubRoadmapService{}
	actor := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	args := json.RawMessage(`{"title":"Shipped feature","status":"shipped","summary":"one-liner","category_id":"` + testRoadmapCatUUID + `"}`)
	res, err := RoadmapItemCreateHandler(roadmapDeps(svc))(context.Background(), args, actor)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("expected success, got %s: %s", res.Status, res.Content)
	}

	in := svc.gotCreate
	if in == nil {
		t.Fatal("service did not receive input")
	}
	if in.Title != "Shipped feature" || in.Status != "shipped" {
		t.Fatalf("unexpected input: %+v", in)
	}
	if in.ActorID != actor {
		t.Fatalf("expected actor %v, got %v", actor, in.ActorID)
	}
	if in.CategoryID == nil || in.CategoryID.String() != testRoadmapCatUUID {
		t.Fatalf("unexpected category_id: %+v", in.CategoryID)
	}
	if in.Summary == nil || *in.Summary != "one-liner" {
		t.Fatalf("expected summary pointer, got %+v", in.Summary)
	}
}

func TestRoadmapItemCreate_InvalidCategoryID(t *testing.T) {
	res, err := RoadmapItemCreateHandler(roadmapDeps(&stubRoadmapService{}))(context.Background(), json.RawMessage(`{"title":"x","category_id":"nope"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "invalid_category_id" {
		t.Fatalf("expected invalid_category_id, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// item.update
// ---------------------------------------------------------------------------

func TestRoadmapItemUpdate_RequiresID(t *testing.T) {
	res, err := RoadmapItemUpdateHandler(roadmapDeps(&stubRoadmapService{}))(context.Background(), json.RawMessage(`{"title":"x"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "missing_id" {
		t.Fatalf("expected missing_id, got %s", got)
	}
}

func TestRoadmapItemUpdate_RejectsStatus(t *testing.T) {
	// Status via update must fail loudly — the REST PATCH silently ignores
	// it, and silently dropping a transition would let an agent believe an
	// item shipped when it did not.
	res, err := RoadmapItemUpdateHandler(roadmapDeps(&stubRoadmapService{}))(context.Background(), json.RawMessage(`{"id":"`+testRoadmapUUID+`","status":"shipped"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "use_set_status" {
		t.Fatalf("expected use_set_status, got %s", got)
	}
}

func TestRoadmapItemUpdate_PassesPatchThrough(t *testing.T) {
	svc := &stubRoadmapService{}

	args := json.RawMessage(`{"id":"` + testRoadmapUUID + `","summary":"updated","category_id":""}`)
	res, err := RoadmapItemUpdateHandler(roadmapDeps(svc))(context.Background(), args, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("expected success, got %s: %s", res.Status, res.Content)
	}

	if svc.gotUpdateID.String() != testRoadmapUUID {
		t.Fatalf("expected id %s, got %s", testRoadmapUUID, svc.gotUpdateID)
	}
	in := svc.gotUpdate
	if in.Summary == nil || *in.Summary != "updated" {
		t.Fatalf("expected summary patch, got %+v", in.Summary)
	}
	// Empty category_id string → non-nil invalid UUID (clear semantics).
	if in.CategoryID == nil || in.CategoryID.Valid {
		t.Fatalf("expected clear-category marker, got %+v", in.CategoryID)
	}
	if in.Title != "" {
		t.Fatalf("absent title should stay empty, got %q", in.Title)
	}
}

// ---------------------------------------------------------------------------
// item.set_status
// ---------------------------------------------------------------------------

func TestRoadmapItemSetStatus_Validation(t *testing.T) {
	h := RoadmapItemSetStatusHandler(roadmapDeps(&stubRoadmapService{}))

	res, _ := h(context.Background(), json.RawMessage(`{"status":"shipped"}`), pgtype.UUID{})
	if got := decodeToolErr(t, res); got != "missing_id" {
		t.Fatalf("expected missing_id, got %s", got)
	}

	res, _ = h(context.Background(), json.RawMessage(`{"id":"`+testRoadmapUUID+`"}`), pgtype.UUID{})
	if got := decodeToolErr(t, res); got != "missing_status" {
		t.Fatalf("expected missing_status, got %s", got)
	}
}

func TestRoadmapItemSetStatus_TransitionErrorCode(t *testing.T) {
	svc := &stubRoadmapService{setStatErr: &RoadmapServiceError{Code: "invalid_status_transition", Message: "pending → shipped"}}

	res, err := RoadmapItemSetStatusHandler(roadmapDeps(svc))(context.Background(), json.RawMessage(`{"id":"`+testRoadmapUUID+`","status":"shipped"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "invalid_status_transition" {
		t.Fatalf("expected invalid_status_transition, got %s", got)
	}
}

func TestRoadmapItemSetStatus_SuccessCarriesActor(t *testing.T) {
	svc := &stubRoadmapService{}
	actor := pgtype.UUID{Bytes: [16]byte{9, 9}, Valid: true}

	res, err := RoadmapItemSetStatusHandler(roadmapDeps(svc))(context.Background(), json.RawMessage(`{"id":"`+testRoadmapUUID+`","status":"shipped"}`), actor)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("expected success, got %s: %s", res.Status, res.Content)
	}
	if svc.gotStatus != "shipped" || svc.gotActorID != actor {
		t.Fatalf("expected shipped for actor %v, got %q/%v", actor, svc.gotStatus, svc.gotActorID)
	}
}

func TestRoadmapHandlers_GenericErrorFallback(t *testing.T) {
	svc := &stubRoadmapService{listErr: errors.New("db down")}

	res, err := RoadmapItemsListHandler(roadmapDeps(svc))(context.Background(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := decodeToolErr(t, res); got != "roadmap_list_failed" {
		t.Fatalf("expected roadmap_list_failed, got %s", got)
	}
}
