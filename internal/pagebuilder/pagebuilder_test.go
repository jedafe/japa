package pagebuilder

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreateBlockInputValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{store: nil}

	// Empty name should fail
	_, err := svc.CreateBlock(t.Context(), testTenantID(), CreateBlockInput{
		Name:      "",
		Slug:      "test",
		Component: "Test.svelte",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}

	// Empty slug should fail
	_, err = svc.CreateBlock(t.Context(), testTenantID(), CreateBlockInput{
		Name:      "Test",
		Slug:      "",
		Component: "Test.svelte",
	})
	if err == nil {
		t.Error("expected error for empty slug")
	}

	// Empty component should fail
	_, err = svc.CreateBlock(t.Context(), testTenantID(), CreateBlockInput{
		Name:      "Test",
		Slug:      "test",
		Component: "",
	})
	if err == nil {
		t.Error("expected error for empty component")
	}
}

func TestCreateTemplateInputValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{store: nil}

	_, err := svc.CreateTemplate(t.Context(), testTenantID(), CreateTemplateInput{
		Name: "",
		Slug: "test",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}

	// Empty slug should be auto-generated from name (no validation error).
	// The store is nil so the call will panic after validation passes;
	// we verify that no *validation* error is returned.
	func() {
		defer func() { recover() }() // swallow the nil-store panic

		_, err = svc.CreateTemplate(t.Context(), testTenantID(), CreateTemplateInput{
			Name: "Test",
			Slug: "",
		})
		if err != nil {
			t.Errorf("expected no validation error for empty slug (auto-generated), got: %v", err)
		}
	}()
}

func TestCreateThemeInputValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{store: nil}

	_, err := svc.CreateTheme(t.Context(), testTenantID(), CreateThemeInput{
		Name: "",
		Slug: "test",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}

	_, err = svc.CreateTheme(t.Context(), testTenantID(), CreateThemeInput{
		Name: "Test",
		Slug: "",
	})
	if err == nil {
		t.Error("expected error for empty slug")
	}
}

func TestCreatePageInputValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{store: nil}

	_, err := svc.CreatePage(t.Context(), testTenantID(), testUserID(), CreatePageInput{
		Title: "",
		Slug:  "test",
	})
	if err == nil {
		t.Error("expected error for empty title")
	}

	// Empty slug should be auto-generated from title (no validation error).
	// The store is nil so the call will panic after validation passes;
	// we verify that no *validation* error is returned.
	func() {
		defer func() { recover() }() // swallow the nil-store panic

		_, err = svc.CreatePage(t.Context(), testTenantID(), testUserID(), CreatePageInput{
			Title: "Test",
			Slug:  "",
		})
		if err != nil {
			t.Errorf("expected no validation error for empty slug (auto-generated), got: %v", err)
		}
	}()
}

func TestUpdateBlockInputValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{store: nil}

	_, err := svc.UpdateBlock(t.Context(), testID(), testTenantID(), UpdateBlockInput{
		Name:      "",
		Component: "Test.svelte",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}

	_, err = svc.UpdateBlock(t.Context(), testID(), testTenantID(), UpdateBlockInput{
		Name:      "Test",
		Component: "",
	})
	if err == nil {
		t.Error("expected error for empty component")
	}
}

func TestUpdateTemplateInputValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{store: nil}

	_, err := svc.UpdateTemplate(t.Context(), testID(), testTenantID(), UpdateTemplateInput{
		Name: "",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestUpdateThemeInputValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{store: nil}

	_, err := svc.UpdateTheme(t.Context(), testID(), testTenantID(), UpdateThemeInput{
		Name: "",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestUpdatePageInputValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{store: nil}

	_, err := svc.UpdatePage(t.Context(), testID(), testTenantID(), UpdatePageInput{
		Title: "",
	})
	if err == nil {
		t.Error("expected error for empty title")
	}
}

func TestDefaultPageStatus(t *testing.T) {
	t.Parallel()

	if PageStatusDraft != "draft" {
		t.Errorf("expected draft, got %s", PageStatusDraft)
	}

	if PageStatusPublished != "published" {
		t.Errorf("expected published, got %s", PageStatusPublished)
	}

	if PageStatusArchived != "archived" {
		t.Errorf("expected archived, got %s", PageStatusArchived)
	}
}

func TestBlockJSONMarshaling(t *testing.T) {
	t.Parallel()

	b := Block{
		Name:          "Hero",
		Slug:          "hero",
		Category:      "layout",
		Component:     "Hero.svelte",
		DefaultConfig: json.RawMessage(`{"title":"Hello"}`),
		ConfigSchema:  json.RawMessage(`{"title":{"type":"string"}}`),
		IsSystem:      false,
		IsEnabled:     true,
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("failed to marshal block: %v", err)
	}

	var decoded Block
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal block: %v", err)
	}

	if decoded.Name != "Hero" {
		t.Errorf("expected name Hero, got %s", decoded.Name)
	}

	if decoded.Slug != "hero" {
		t.Errorf("expected slug hero, got %s", decoded.Slug)
	}
}

func TestPageJSONMarshaling(t *testing.T) {
	t.Parallel()

	p := Page{
		Title:  "Landing Page",
		Slug:   "landing",
		Blocks: json.RawMessage(`[{"type":"hero","config":{}}]`),
		Meta:   json.RawMessage(`{"seo_title":"Landing"}`),
		Status: PageStatusDraft,
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal page: %v", err)
	}

	var decoded Page
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal page: %v", err)
	}

	if decoded.Title != "Landing Page" {
		t.Errorf("expected title Landing Page, got %s", decoded.Title)
	}

	if decoded.Status != PageStatusDraft {
		t.Errorf("expected status draft, got %s", decoded.Status)
	}
}

// testTenantID returns a test tenant UUID.
func testTenantID() pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan("00000000-0000-0000-0000-000000000001")

	return id
}

// testUserID returns a test user UUID.
func testUserID() pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan("00000000-0000-0000-0000-000000000002")

	return id
}

// testID returns a test resource UUID.
func testID() pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan("00000000-0000-0000-0000-000000000003")

	return id
}
