package wiki

// Product instance CRUD (§80 Product Domain) — admin write surface on
// products + product_providers, sibling to the read-side directory.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ProductProviderInput is one provider link in the create/update body.
type ProductProviderInput struct {
	ProviderID string `json:"provider_id"`
	PriceFrom  string `json:"price_from"`
	OfferURL   string `json:"offer_url"`
	Position   int32  `json:"position"`
}

// ProductFieldValueInput is one custom field value (cfv, entity_type='product')
// in the create/update body — the typed long tail resolved via the product
// type's bound field groups (§80).
type ProductFieldValueInput struct {
	FieldID     string           `json:"field_id"`
	ValueText   *string          `json:"value_text"`
	ValueNumber *float64         `json:"value_number"`
	ValueBool   *bool            `json:"value_bool"`
	ValueDate   *string          `json:"value_date"` // RFC 3339
	ValueJSON   *json.RawMessage `json:"value_json"`
}

// ProductInput is the create/update body for a product. On PATCH, Providers
// and DealIDs are full-replace sets when non-nil (nil = untouched, [] = clear
// all); CustomFields upserts only the fields sent, ClearFieldIDs deletes rows.
type ProductInput struct {
	TypeSlug      string                   `json:"type_slug"` // product_types.slug — slug, never uuid
	Name          string                   `json:"name"`
	Slug          string                   `json:"slug"`
	Tagline       *string                  `json:"tagline"`
	WebsiteURL    *string                  `json:"website_url"`
	PricingModel  *string                  `json:"pricing_model"`
	PriceFrom     *float64                 `json:"price_from"`
	Providers     []ProductProviderInput   `json:"providers"`
	DealIDs       []string                 `json:"deal_ids"`
	CustomFields  []ProductFieldValueInput `json:"custom_fields"`
	ClearFieldIDs []string                 `json:"clear_field_ids"`
}

func (s *Service) productTypeID(ctx context.Context, typeSlug string) (pgtype.UUID, error) {
	pt, err := s.store.read.GetProductTypeBySlug(ctx, typeSlug)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pt.ID, nil
}

func numPtr(f *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if f != nil {
		// pgtype.Numeric.Scan only accepts strings — scanning a raw
		// float64 fails and silently produces NULL.
		_ = n.Scan(fmt.Sprintf("%f", *f))
	}
	return n
}

func uuidOf(id string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(id)
	return u
}

// CreateProduct inserts a product under a type slug, plus its provider links,
// deal links and custom field values in one tx (§80 product management).
func (s *Service) CreateProduct(ctx context.Context, in ProductInput) (string, error) {
	if in.TypeSlug == "" || in.Slug == "" || in.Name == "" {
		return "", fmt.Errorf("type_slug, slug and name are required")
	}
	typeID, err := s.productTypeID(ctx, in.TypeSlug)
	if err != nil {
		return "", fmt.Errorf("product type %q not found", in.TypeSlug)
	}
	tx, err := s.store.pool.Writer.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.store.write.WithTx(tx)

	row, err := q.CreateProduct(ctx, gen.CreateProductParams{
		ProductTypeID: typeID, Slug: in.Slug, Name: in.Name,
		Tagline: in.Tagline, WebsiteUrl: in.WebsiteURL,
		PricingModel: in.PricingModel, PriceFrom: numPtr(in.PriceFrom),
	})
	if err != nil {
		return "", err
	}
	if err := s.syncProductLinks(ctx, q, row.ID, in); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return row.Slug, nil
}

var errInvalidID = errors.New("invalid id")

// validUUID parses an id into a pgtype.UUID, failing with errInvalidID on
// malformed input (uuidOf alone would silently produce a zero UUID → FK error).
func validUUID(id string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(id); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("%w: %q", errInvalidID, id)
	}
	return u, nil
}

// syncProductLinks writes provider links, deal links and custom field values
// for a product inside q's tx.
func (s *Service) syncProductLinks(ctx context.Context, q *gen.Queries, productID pgtype.UUID, in ProductInput) error {
	for i, pv := range in.Providers {
		pid, err := validUUID(pv.ProviderID)
		if err != nil {
			return err
		}
		var pf pgtype.Numeric
		if pv.PriceFrom != "" {
			_ = pf.Scan(pv.PriceFrom)
		}
		var ou *string
		if pv.OfferURL != "" {
			ou = &pv.OfferURL
		}
		pos := pv.Position
		if pos == 0 {
			pos = int32(i)
		}
		if err := q.UpsertProductProvider(ctx, gen.UpsertProductProviderParams{
			ProductID: productID, ProviderID: pid,
			PriceFrom: pf, OfferUrl: ou, Position: pos,
		}); err != nil {
			return fmt.Errorf("link provider: %w", err)
		}
	}
	for i, did := range in.DealIDs {
		cid, err := validUUID(did)
		if err != nil {
			return err
		}
		if err := q.UpsertProductDeal(ctx, gen.UpsertProductDealParams{
			ProductID: productID, CouponID: cid, Position: int32(i),
		}); err != nil {
			return fmt.Errorf("link deal: %w", err)
		}
	}
	return s.writeProductFieldValues(ctx, q, productID, in.CustomFields, in.ClearFieldIDs)
}

// writeProductFieldValues upserts cfv rows (entity_type='product') and deletes
// any field ids in clearIDs.
func (s *Service) writeProductFieldValues(ctx context.Context, q *gen.Queries, productID pgtype.UUID, fields []ProductFieldValueInput, clearIDs []string) error {
	for _, f := range fields {
		fid, err := validUUID(f.FieldID)
		if err != nil {
			return err
		}
		var num pgtype.Numeric
		if f.ValueNumber != nil {
			_ = num.Scan(fmt.Sprintf("%f", *f.ValueNumber))
		}
		var dt pgtype.Timestamptz
		if f.ValueDate != nil {
			_ = dt.Scan(*f.ValueDate)
		}
		var vj []byte
		if f.ValueJSON != nil {
			vj = *f.ValueJSON
		}
		if _, err := q.UpsertCustomFieldValue(ctx, gen.UpsertCustomFieldValueParams{
			FieldID: fid, EntityType: "product", EntityID: productID,
			ValueText: f.ValueText, ValueNumber: num, ValueBool: f.ValueBool,
			ValueDate: dt, ValueJson: vj,
		}); err != nil {
			return fmt.Errorf("set field value: %w", err)
		}
	}
	for _, id := range clearIDs {
		cfid, err := validUUID(id)
		if err != nil {
			return err
		}
		if err := q.DeleteCustomFieldValue(ctx, gen.DeleteCustomFieldValueParams{
			FieldID: cfid, EntityType: "product", EntityID: productID,
		}); err != nil {
			return fmt.Errorf("clear field value: %w", err)
		}
	}
	return nil
}

// UpdateProduct patches mutable fields on a product (by slug).
func (s *Service) UpdateProduct(ctx context.Context, slug string, in ProductInput) error {
	var name *string
	if in.Name != "" {
		name = &in.Name
	}
	// nil = keep (COALESCE), "" = clear (clear_fields) — PATCH can both
	// preserve and clear per key. price_from has no clear path (zero is a
	// valid price; clearing needs an explicit admin op).
	clear := []string{}
	for _, kv := range []struct {
		key string
		val *string
	}{{"tagline", in.Tagline}, {"website_url", in.WebsiteURL}, {"pricing_model", in.PricingModel}} {
		if kv.val != nil && *kv.val == "" {
			clear = append(clear, kv.key)
		}
	}
	tx, err := s.store.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.store.write.WithTx(tx)

	var typeID pgtype.UUID
	if in.TypeSlug != "" {
		var terr error
		typeID, terr = s.productTypeID(ctx, in.TypeSlug)
		if terr != nil {
			return fmt.Errorf("product type %q not found: %w", in.TypeSlug, terr)
		}
	}
	if err := q.UpdateProduct(ctx, gen.UpdateProductParams{
		Name: name, ProductTypeID: typeID, Tagline: in.Tagline, WebsiteUrl: in.WebsiteURL,
		PricingModel: in.PricingModel, PriceFrom: numPtr(in.PriceFrom), Slug: slug,
		ClearFields: clear,
	}); err != nil {
		return err
	}
	// AdminGetProductBySlug includes soft-deleted rows — a PATCH on a deleted
	// product still works (edit-then-restore flow).
	p, err := q.AdminGetProductBySlug(ctx, slug)
	if err != nil {
		return err
	}
	// Providers full-replace when the key is present (nil = untouched).
	if in.Providers != nil {
		existing, err := q.ListProductProviders(ctx, p.ID)
		if err != nil {
			return err
		}
		keep := map[string]bool{}
		for _, pv := range in.Providers {
			keep[pv.ProviderID] = true
		}
		for _, row := range existing {
			if !keep[row.ProviderID.String()] {
				if err := q.RemoveProductProvider(ctx, gen.RemoveProductProviderParams{
					ProductID: p.ID, ProviderID: row.ProviderID,
				}); err != nil {
					return fmt.Errorf("unlink provider: %w", err)
				}
			}
		}
	}
	// Deals full-replace when the key is present.
	if in.DealIDs != nil {
		keep := make([]pgtype.UUID, 0, len(in.DealIDs))
		for _, d := range in.DealIDs {
			keep = append(keep, uuidOf(d))
		}
		if err := q.DeleteProductDealsNotIn(ctx, gen.DeleteProductDealsNotInParams{
			ProductID: p.ID, KeepIds: keep,
		}); err != nil {
			return fmt.Errorf("prune deals: %w", err)
		}
	}
	if err := s.syncProductLinks(ctx, q, p.ID, in); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteProduct soft-deletes a product by slug.
func (s *Service) DeleteProduct(ctx context.Context, slug string) error {
	n, err := s.store.write.DeleteProduct(ctx, slug)
	if err == nil && n == 0 {
		return pgx.ErrNoRows
	}
	return err
}

// SetProductProvider links/unlinks a provider with optional price+offer.
func (s *Service) SetProductProvider(ctx context.Context, productSlug string, providerID, priceStr, offerURL string, position int32, unlink bool) error {
	p, err := s.store.read.GetProductBySlug(ctx, productSlug)
	if err != nil {
		return err
	}
	pid, err := validUUID(providerID)
	if err != nil {
		return err
	}
	if unlink {
		return s.store.write.RemoveProductProvider(ctx, gen.RemoveProductProviderParams{ProductID: p.ID, ProviderID: pid})
	}
	var pf pgtype.Numeric
	if priceStr != "" {
		_ = pf.Scan(priceStr)
	}
	var ou *string
	if offerURL != "" {
		ou = &offerURL
	}
	return s.store.write.UpsertProductProvider(ctx, gen.UpsertProductProviderParams{
		ProductID: p.ID, ProviderID: pid, PriceFrom: pf, OfferUrl: ou, Position: position,
	})
}

// RestoreProduct clears deleted_at on a soft-deleted product.
func (s *Service) RestoreProduct(ctx context.Context, slug string) error {
	n, err := s.store.write.RestoreProduct(ctx, slug)
	if err == nil && n == 0 {
		return pgx.ErrNoRows
	}
	return err
}

// HardDeleteProduct permanently removes a soft-deleted product — the
// ?force=true path gated by admin.hard_delete (Rule 11).
func (s *Service) HardDeleteProduct(ctx context.Context, slug string) error {
	n, err := s.store.write.HardDeleteProduct(ctx, slug)
	if err == nil && n == 0 {
		return pgx.ErrNoRows
	}
	return err
}

// AdminListProducts is the DataTable contract list: type/q/status filters +
// allow-listed sort + offset pagination.
var ilikeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (s *Service) AdminListProducts(ctx context.Context, typeSlug, q, status, sort string, limit, offset int32) ([]gen.AdminListProductsRow, int64, error) {
	if status == "" {
		status = "active"
	}
	q = ilikeEscaper.Replace(q)
	sortKey := ""
	switch sort {
	case "name:asc", "name:desc", "created_at:asc":
		sortKey = sort
	}
	rows, err := s.store.read.AdminListProducts(ctx, gen.AdminListProductsParams{
		TypeSlug: typeSlug, Q: q, Status: status, SortKey: sortKey,
		LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.read.AdminCountProducts(ctx, gen.AdminCountProductsParams{
		TypeSlug: typeSlug, Q: q, Status: status,
	})
	return rows, total, err
}

// ProductDetail is the admin edit payload — core row + provider links +
// explicit deal links + current custom field values.
type ProductDetail struct {
	Product   gen.AdminGetProductBySlugRow                 `json:"product"`
	Providers []map[string]any                             `json:"providers"`
	Deals     []gen.ListProductDealsRow                    `json:"deals"`
	Fields    []gen.CustomFieldValue                      `json:"field_values"`
}

// AdminGetProduct returns the full product detail (deleted rows included so
// the admin edit page can offer restore).
func (s *Service) AdminGetProduct(ctx context.Context, slug string) (*ProductDetail, error) {
	p, err := s.store.read.AdminGetProductBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	d := &ProductDetail{Product: p}
	provs, err := s.store.read.ListProductProviders(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	for _, pp := range provs {
		d.Providers = append(d.Providers, map[string]any{
			"provider_id": pp.ProviderID.String(), "provider_slug": pp.ProviderSlug,
			"name": pp.Name, "price_from": pp.PriceFrom, "offer_url": pp.OfferUrl,
			"has_deal": pp.HasDeal,
		})
	}
	d.Deals, err = s.store.read.ListProductDeals(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	// ListCustomFieldValuesByEntity (not the WithDefs variant — that one
	// filters show_in_frontend, which would hide admin-only fields).
	d.Fields, err = s.store.read.ListCustomFieldValuesByEntity(ctx,
		gen.ListCustomFieldValuesByEntityParams{EntityType: "product", EntityID: p.ID})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// SetProductDeal links/unlinks a deal (coupon) on a product — the explicit
// featured-deals list (provider-derived has_deal stays untouched).
func (s *Service) SetProductDeal(ctx context.Context, productSlug string, couponID string, position int32, unlink bool) error {
	p, err := s.store.read.GetProductBySlug(ctx, productSlug)
	if err != nil {
		return err
	}
	cid, err := validUUID(couponID)
	if err != nil {
		return err
	}
	if unlink {
		return s.store.write.RemoveProductDeal(ctx, gen.RemoveProductDealParams{ProductID: p.ID, CouponID: cid})
	}
	return s.store.write.UpsertProductDeal(ctx, gen.UpsertProductDealParams{
		ProductID: p.ID, CouponID: cid, Position: position,
	})
}

// ProductTypeInput is the PATCH body for a product type. Categories are
// append-only — new categories provision group + assignment + pool row.
type ProductTypeInput struct {
	Label      *string               `json:"label"`
	Icon       *string               `json:"icon"`      // "" clears
	IsEnabled  *bool                 `json:"is_enabled"`
	Config     json.RawMessage       `json:"config"`
	Categories []ProductTypeCategory `json:"categories"`
}

func (s *Service) AdminListProductTypes(ctx context.Context, q string, limit, offset int32) ([]gen.AdminListProductTypesRow, int64, error) {
	rows, err := s.store.read.AdminListProductTypes(ctx, gen.AdminListProductTypesParams{
		Q: ilikeEscaper.Replace(q), LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.read.AdminCountProductTypes(ctx, ilikeEscaper.Replace(q))
	return rows, total, err
}

// ProductTypeDetail carries a type plus its bound field groups and their
// fields — everything the admin type page renders in one payload.
type ProductTypeDetail struct {
	Type   gen.GetProductTypeBySlugRow `json:"type"`
	Groups []FieldSchemaGroup          `json:"groups"`
}

func (s *Service) AdminGetProductType(ctx context.Context, slug string) (*ProductTypeDetail, error) {
	pt, err := s.store.read.GetProductTypeBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	groups, err := s.ProductFieldSchema(ctx, slug)
	if err != nil {
		return nil, err
	}
	return &ProductTypeDetail{Type: pt, Groups: groups}, nil
}

// UpdateProductType patches label/icon/is_enabled/config and appends new
// categories — each new category provisions its field group + cfga assignment
// + comparison-pool row, mirroring CreateProductType's chain.
func (s *Service) UpdateProductType(ctx context.Context, slug string, in ProductTypeInput) error {
	tx, err := s.store.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.store.write.WithTx(tx)

	clearFields := []string{}
	if in.Icon != nil && *in.Icon == "" {
		clearFields = append(clearFields, "icon")
	}
	row, err := q.UpdateProductType(ctx, gen.UpdateProductTypeParams{
		Label: in.Label, Icon: in.Icon, IsEnabled: in.IsEnabled,
		Config: in.Config, Slug: slug, ClearFields: clearFields,
	})
	if err != nil {
		return fmt.Errorf("update product type: %w", err)
	}
	label := row.Label

	if len(in.Categories) > 0 {
		existing, err := q.ListAssignmentsByEntityClass(ctx, slug)
		if err != nil {
			return fmt.Errorf("list type groups: %w", err)
		}
		pos := int32(len(existing))
		// The comparison type keyed by the same slug carries the pool rows.
		var ctID pgtype.UUID
		if ct, err := q.GetComparisonTypeBySlug(ctx, slug); err == nil {
			ctID = ct.ID
		}
		for _, cat := range in.Categories {
			gslug := "wiki_" + strings.ReplaceAll(slug, "-", "_") + "_" + strings.ReplaceAll(cat.Slug, "-", "_")
			g, err := q.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{
				Name: label + " — " + cat.Label, Slug: gslug, IsSystem: true,
			})
			if err != nil {
				return fmt.Errorf("creating field group %s: %w", gslug, err)
			}
			if _, err := q.CreateAssignment(ctx, gen.CreateAssignmentParams{
				GroupID: g.ID, EntityClass: slug, Position: pos,
			}); err != nil {
				return fmt.Errorf("assigning group %s: %w", gslug, err)
			}
			if ctID.Valid {
				catSlug := cat.Slug
				lbl := cat.Label
				if err := q.CreateComparisonTypeFieldGroup(ctx, gen.CreateComparisonTypeFieldGroupParams{
					TypeID: ctID, GroupID: g.ID,
					Category: &catSlug, Label: &lbl, Position: pos,
				}); err != nil {
					return fmt.Errorf("seeding pool for %s: %w", gslug, err)
				}
			}
			pos++
		}
	}
	return tx.Commit(ctx)
}

var errProductTypeInUse = errors.New("product type has products")

// DeleteProductType removes a type only when no live products reference it —
// products.product_type_id is ON DELETE RESTRICT so the check returns a clean
// error instead of a FK violation.
func (s *Service) DeleteProductType(ctx context.Context, slug string) error {
	pt, err := s.store.read.GetProductTypeBySlug(ctx, slug)
	if err != nil {
		return err
	}
	n, err := s.store.read.AdminCountProductsByType(ctx, pt.ID)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %d", errProductTypeInUse, n)
	}
	_, err = s.store.write.DeleteProductType(ctx, slug)
	return err
}

// FieldSchemaField is one custom field in a type's schema.
type FieldSchemaField struct {
	ID           string          `json:"id"`
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	FieldType    string          `json:"field_type"`
	Options      json.RawMessage `json:"options,omitempty"`
	Placeholder  *string         `json:"placeholder,omitempty"`
	DefaultValue *string         `json:"default_value,omitempty"`
	IsRequired   bool            `json:"is_required"`
	Position     int32           `json:"position"`
	Section      *string         `json:"section,omitempty"`
}

// FieldSchemaGroup is one bound field group + its fields.
type FieldSchemaGroup struct {
	ID       string             `json:"id"`
	Slug     string             `json:"slug"`
	Name     string             `json:"name"`
	Position int32              `json:"position"`
	Fields   []FieldSchemaField `json:"fields"`
}

// ProductFieldSchema resolves the field groups bound to a product type slug
// (cfga entity_class = slug) with their fields — the /products/new form schema.
func (s *Service) ProductFieldSchema(ctx context.Context, typeSlug string) ([]FieldSchemaGroup, error) {
	rows, err := s.store.read.ListProductTypeGroupsWithFields(ctx, typeSlug)
	if err != nil {
		return nil, err
	}
	out := []FieldSchemaGroup{}
	idx := map[string]int{}
	for _, r := range rows {
		gid := r.GroupID.String()
		i, ok := idx[gid]
		if !ok {
			out = append(out, FieldSchemaGroup{
				ID: gid, Slug: r.GroupSlug, Name: r.GroupName, Position: r.GroupPosition, Fields: []FieldSchemaField{},
			})
			i = len(out) - 1
			idx[gid] = i
		}
		if r.FieldID.Valid {
			out[i].Fields = append(out[i].Fields, FieldSchemaField{
				ID: r.FieldID.String(), Key: derefStr(r.Key), Label: derefStr(r.Label),
				FieldType: derefStr(r.FieldType), Options: r.Options,
				Placeholder: r.Placeholder, DefaultValue: r.DefaultValue,
				IsRequired: r.IsRequired != nil && *r.IsRequired,
				Position:   derefInt32(r.FieldPosition), Section: r.Section,
			})
		}
	}
	return out, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt32(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

// --- handlers ---

func (h *Handlers) HandleCreateProduct(w http.ResponseWriter, r *http.Request) {
	var in ProductInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if in.TypeSlug == "" || in.Slug == "" || in.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "type_slug, slug and name are required")
		return
	}
	slug, err := h.service.CreateProduct(r.Context(), in)
	if err != nil {
		var pge *pgconn.PgError
		if errors.As(err, &pge) && pge.Code == "23505" {
			errpkg.WriteError(w, http.StatusConflict, "slug_conflict", "A product with this slug already exists")
			return
		}
		if errors.Is(err, errInvalidID) || errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}
		slog.Error("wiki: create product failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create product")
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"slug": slug}})
}

func (h *Handlers) HandleUpdateProduct(w http.ResponseWriter, r *http.Request) {
	var in ProductInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if err := h.service.UpdateProduct(r.Context(), chi.URLParam(r, "slug"), in); err != nil {
		writeErr(w, err, "update_failed", "Failed to update product")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

func (h *Handlers) HandleDeleteProduct(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	// ?force=true permanently removes an already-soft-deleted row — the
	// admin.hard_delete node is enforced by rbac.RequireForceDelete (Rule 11).
	if r.URL.Query().Get("force") == "true" {
		if err := h.service.HardDeleteProduct(r.Context(), slug); err != nil {
			writeErr(w, err, "delete_failed", "Failed to delete product")
			return
		}
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
		return
	}
	if err := h.service.DeleteProduct(r.Context(), slug); err != nil {
		writeErr(w, err, "delete_failed", "Failed to delete product")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleAdminListProducts serves GET /admin/products — the DataTable contract
// (?page&limit&sort&dir&q&type&status=active|deleted|all).
func (h *Handlers) HandleAdminListProducts(w http.ResponseWriter, r *http.Request) {
	qp := r.URL.Query()
	sort := qp.Get("sort")
	if sort != "" {
		dir := qp.Get("dir")
		if dir == "" {
			dir = "asc"
		}
		sort += ":" + dir
	}
	page, _ := strconv.Atoi(qp.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(qp.Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 25
	}
	rows, total, err := h.service.AdminListProducts(r.Context(),
		qp.Get("type"), qp.Get("q"), qp.Get("status"), sort, int32(limit), int32((page-1)*limit))
	if err != nil {
		slog.Error("wiki: admin list products failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list products")
		return
	}
	if rows == nil {
		rows = []gen.AdminListProductsRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows, "total": total, "page": page, "page_size": limit,
	})
}

// HandleAdminGetProduct serves GET /admin/products/{slug} — the edit payload.
func (h *Handlers) HandleAdminGetProduct(w http.ResponseWriter, r *http.Request) {
	d, err := h.service.AdminGetProduct(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "product_not_found", "Product not found")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": d})
}

// HandleRestoreProduct serves POST /admin/products/{slug}/restore.
func (h *Handlers) HandleRestoreProduct(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RestoreProduct(r.Context(), chi.URLParam(r, "slug")); err != nil {
		writeErr(w, err, "restore_failed", "Failed to restore product")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleBulkProducts serves POST /admin/products/bulk — {action, ids[]} where
// ids are product slugs (products are slug-addressed). Per-slug failures are
// collected, not fatal.
func (h *Handlers) HandleBulkProducts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string   `json:"action"`
		IDs    []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	failed := []string{}
	for _, slug := range body.IDs {
		var err error
		switch body.Action {
		case "delete":
			err = h.service.DeleteProduct(r.Context(), slug)
		case "restore":
			err = h.service.RestoreProduct(r.Context(), slug)
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "unknown_action", "Unsupported bulk action")
			return
		}
		if err != nil {
			failed = append(failed, slug)
		}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"failed": failed}})
}

// HandleSetProductDeal serves PUT+DELETE
// /admin/products/{slug}/deals/{coupon_id} — the explicit deal link.
func (h *Handlers) HandleSetProductDeal(w http.ResponseWriter, r *http.Request) {
	couponID := chi.URLParam(r, "coupon_id")
	slug := chi.URLParam(r, "slug")

	unlink := r.Method == http.MethodDelete
	var pos int32
	if !unlink {
		var body struct {
			Position int32 `json:"position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		pos = body.Position
	}
	if err := h.service.SetProductDeal(r.Context(), slug, couponID, pos, unlink); err != nil {
		writeErr(w, err, "set_failed", "Failed to set product deal")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleAdminProductTypes serves GET /admin/product-types — all types incl.
// disabled, with live product counts.
func (h *Handlers) HandleAdminProductTypes(w http.ResponseWriter, r *http.Request) {
	qp := r.URL.Query()
	page, _ := strconv.Atoi(qp.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(qp.Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 200
	}
	rows, total, err := h.service.AdminListProductTypes(r.Context(), qp.Get("q"), int32(limit), int32((page-1)*limit))
	if err != nil {
		slog.Error("wiki: list product types failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list product types")
		return
	}
	if rows == nil {
		rows = []gen.AdminListProductTypesRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows, "total": total, "page": page, "page_size": limit,
	})
}

// HandleAdminGetProductType serves GET /admin/product-types/{slug} — type +
// bound field groups + fields.
func (h *Handlers) HandleAdminGetProductType(w http.ResponseWriter, r *http.Request) {
	d, err := h.service.AdminGetProductType(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Product type not found")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": d})
}

// writeErr maps sentinel errors to the standard envelope — ErrNoRows → 404,
// errInvalidID → 400 — everything else → 500 with the given code.
func writeErr(w http.ResponseWriter, err error, code, msg string) {
	if errors.Is(err, pgx.ErrNoRows) {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Not found")
		return
	}
	if errors.Is(err, errInvalidID) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	slog.Error("wiki: "+code, "error", err)
	errpkg.WriteError(w, http.StatusInternalServerError, code, msg)
}

// HandleUpdateProductType serves PATCH /admin/product-types/{slug}.
func (h *Handlers) HandleUpdateProductType(w http.ResponseWriter, r *http.Request) {
	var in ProductTypeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if err := h.service.UpdateProductType(r.Context(), chi.URLParam(r, "slug"), in); err != nil {
		writeErr(w, err, "update_failed", "Failed to update product type")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleDeleteProductType serves DELETE /admin/product-types/{slug} — 409 when
// live products still reference the type.
func (h *Handlers) HandleDeleteProductType(w http.ResponseWriter, r *http.Request) {
	err := h.service.DeleteProductType(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		if errors.Is(err, errProductTypeInUse) {
			errpkg.WriteError(w, http.StatusConflict, "type_has_products", "Product type still has products — remove or retype them first")
			return
		}
		writeErr(w, err, "delete_failed", "Failed to delete product type")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleListProductDeals serves GET /products/deals?provider_ids=a,b — the
// deal-picker source for the product forms (auth-only; no coupons.manage).
func (h *Handlers) HandleListProductDeals(w http.ResponseWriter, r *http.Request) {
	var ids []pgtype.UUID
	if raw := r.URL.Query().Get("provider_ids"); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			id, err := validUUID(strings.TrimSpace(s))
			if err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
				return
			}
			ids = append(ids, id)
		}
	}
	if ids == nil {
		ids = []pgtype.UUID{}
	}
	rows, err := h.service.store.read.ListCouponsForProviders(r.Context(), ids)
	if err != nil {
		slog.Error("wiki: list deals failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list deals")
		return
	}
	if rows == nil {
		rows = []gen.ListCouponsForProvidersRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// HandleProductFieldSchema serves GET /products/types/{slug}/field-schema —
// the bound field groups + fields for the product create/edit forms.
func (h *Handlers) HandleProductFieldSchema(w http.ResponseWriter, r *http.Request) {
	groups, err := h.service.ProductFieldSchema(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		slog.Error("wiki: product field schema failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to load field schema")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"groups": groups}})
}

// offeringInput is one university's offering field set in the wiki course
// create body — empty fields fall back to the shared top-level values, so
// per-university tuition/mode/etc. can differ like the admin form.
type offeringInput struct {
	UniversitySlug    string   `json:"university_slug"`
	Tuition           *float64 `json:"tuition"`
	Currency          string   `json:"currency"`
	DurationYears     *float64 `json:"duration_years"`
	StudyMode         string   `json:"study_mode"`
	IntakeMonths      []string `json:"intake_months"`
	EntryRequirements string   `json:"entry_requirements"`
	Accreditation     string   `json:"accreditation"`
}

// HandleCreateEntityInstance serves POST /wiki/e/{type_slug} — creates an
// entity on its canonical listing (§80): product types → products,
// 'program' → university_course_catalog, 'course' → offerings (with
// university + program resolution). Pages lazy-materialize on first view.
func (h *Handlers) HandleCreateEntityInstance(w http.ResponseWriter, r *http.Request) {
	typeSlug := chi.URLParam(r, "type_slug")
	ctx := r.Context()

	var body struct {
		Slug            string   `json:"slug"`
		Name            string   `json:"name"`
		Title           string   `json:"title"`
		Language        string   `json:"language"`
		UniversitySlug  string   `json:"university_slug"`
		UniversitySlugs []string `json:"university_slugs"`
		ProgramSlug     string   `json:"program_slug"`
		HostSlug        string   `json:"host_slug"`
		DestinationSlug string   `json:"destination_slug"`
		VisaCategory    string   `json:"visa_category"`
		CountryCode     string   `json:"country_code"`
		Country         string   `json:"country"`
		SourceURL       string   `json:"source_url"`
		GovSourceURL    string   `json:"gov_source_url"`
		Tagline         string   `json:"tagline"`
		WebsiteURL      string   `json:"website_url"`
		PricingModel    string   `json:"pricing_model"`
		PriceFrom       *float64 `json:"price_from"`
		// §39 catalog fields — same set as admin /universities/course-catalog.
		Description          string   `json:"description"`
		DegreeType           string   `json:"degree_type"`
		Department           string   `json:"department"`
		FieldOfStudy         string   `json:"field_of_study"`
		DefaultDurationYears *float64 `json:"default_duration_years"`
		// §39 offering fields — same set as admin /universities/course-offerings.
		Tuition           *float64 `json:"tuition"`
		Currency          string   `json:"currency"`
		DurationYears     *float64 `json:"duration_years"`
		StudyMode         string   `json:"study_mode"`
		IntakeMonths      []string `json:"intake_months"`
		EntryRequirements string   `json:"entry_requirements"`
		Accreditation     string   `json:"accreditation"`
		// Per-university offering field sets — alternative to
		// university_slugs + the shared fields above.
		Offerings []offeringInput `json:"offerings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	name := body.Name
	if name == "" {
		name = body.Title
	}

	// Table-backed kinds. Products are not wiki entities — they are
	// created via the admin product CRUD (POST /admin/wiki/products,
	// reachable from /products) and never resolve here.
	kind, err := h.service.store.read.GetWikiEntityKind(ctx, typeSlug)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Entity type not found")
		return
	}

	if name == "" && kind.EntityKind != "course" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	// Slug is optional for program + course — auto-generated like the
	// admin forms (title-derived for catalog rows, {university}-{course}
	// for offerings). Other kinds still require an explicit slug.
	if kind.EntityKind == "program" && body.Slug == "" {
		body.Slug = entitySlug(name)
	}
	if body.Slug == "" && kind.EntityKind != "course" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	href := "/wiki/e/" + typeSlug + "/" + body.Slug + "/overview"
	out := map[string]any{"slug": body.Slug, "href": href}

	switch kind.EntityKind {
	case "program":
		if _, err := h.service.store.write.CreateProgramForWiki(ctx, gen.CreateProgramForWikiParams{
			Title: name, Slug: body.Slug,
			Description:          strPtr(body.Description),
			DegreeType:           strPtr(body.DegreeType),
			Department:           strPtr(body.Department),
			FieldOfStudy:         strPtr(body.FieldOfStudy),
			Language:             strPtr(body.Language),
			DefaultDurationYears: numPtr(body.DefaultDurationYears),
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}
	case "course":
		// Universities come either as university_slugs (+ shared field
		// set) or as a per-university offerings array — the wiki form
		// sends offerings so tuition/mode/etc. can differ across unis.
		uniSlugs := body.UniversitySlugs
		if len(uniSlugs) == 0 && body.UniversitySlug != "" {
			uniSlugs = []string{body.UniversitySlug}
		}
		if len(uniSlugs) == 0 {
			for _, o := range body.Offerings {
				if o.UniversitySlug != "" {
					uniSlugs = append(uniSlugs, o.UniversitySlug)
				}
			}
		}
		if len(uniSlugs) == 0 || body.ProgramSlug == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_parents", "university_slugs and program_slug are required for courses")
			return
		}
		prog, err := h.service.store.read.GetProgramForWiki(ctx, body.ProgramSlug)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "program_not_found", "Program not found")
			return
		}
		offeringFor := func(uslug string) *offeringInput {
			for i := range body.Offerings {
				if body.Offerings[i].UniversitySlug == uslug {
					return &body.Offerings[i]
				}
			}
			return nil
		}
		created := []string{}
		skipped := []string{}
		for _, uslug := range uniSlugs {
			uni, err := h.service.store.read.GetSpaceBySlugKind(ctx, gen.GetSpaceBySlugKindParams{Slug: uslug, TypeSlug: "education"})
			if err != nil {
				skipped = append(skipped, uslug)
				continue // skip unknown universities rather than fail the batch
			}
			// One offering per selected uni. Explicit slug → uni-prefixed
			// when multi (unilag-bsc-cs); omitted → {university}-{course}
			// like the admin form's auto-generation.
			oslug := body.Slug
			if oslug == "" {
				oslug = uslug + "-" + body.ProgramSlug
			} else if len(uniSlugs) > 1 {
				oslug = uslug + "-" + oslug
			}
			params := gen.CreateOfferingForWikiParams{
				CourseID: prog.ID, UniversityID: uni, Slug: oslug,
				Tuition:           numPtr(body.Tuition),
				Currency:          strPtr(body.Currency),
				DurationYears:     numPtr(body.DurationYears),
				StudyMode:         strPtr(body.StudyMode),
				IntakeMonths:      body.IntakeMonths,
				EntryRequirements: strPtr(body.EntryRequirements),
				Accreditation:     strPtr(body.Accreditation),
			}
			if o := offeringFor(uslug); o != nil {
				if o.Tuition != nil {
					params.Tuition = numPtr(o.Tuition)
				}
				if o.Currency != "" {
					params.Currency = strPtr(o.Currency)
				}
				if o.DurationYears != nil {
					params.DurationYears = numPtr(o.DurationYears)
				}
				if o.StudyMode != "" {
					params.StudyMode = strPtr(o.StudyMode)
				}
				if o.IntakeMonths != nil {
					params.IntakeMonths = o.IntakeMonths
				}
				if o.EntryRequirements != "" {
					params.EntryRequirements = strPtr(o.EntryRequirements)
				}
				if o.Accreditation != "" {
					params.Accreditation = strPtr(o.Accreditation)
				}
			}
			if _, err := h.service.store.write.CreateOfferingForWiki(ctx, params); err == nil {
				created = append(created, oslug)
			} else {
				skipped = append(skipped, uslug) // existing slug or other conflict
			}
		}
		if len(skipped) > 0 {
			out["skipped"] = skipped
		}
		if len(created) == 0 {
			errpkg.WriteError(w, http.StatusConflict, "no_offerings_created", "No offerings created — universities not found or slugs already exist")
			return
		}
		out["created"] = created
		out["slug"] = created[0]
		out["href"] = "/wiki/e/" + typeSlug + "/" + created[0] + "/overview"

	case "embassy":
		host, err := h.service.store.read.GetSpaceBySlugKind(ctx, gen.GetSpaceBySlugKindParams{Slug: body.HostSlug, TypeSlug: "country"})
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "host_not_found", "Host country not found")
			return
		}
		if _, err := h.service.store.write.CreateEmbassyForWiki(ctx, gen.CreateEmbassyForWikiParams{
			Name: name, Slug: body.Slug, HostSpaceID: host,
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}

	case "bank":
		if body.CountryCode != "" && (len(body.CountryCode) != 2 || !isAlpha(body.CountryCode)) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_country_code", "Country code must be 2 letters (ISO 3166-1)")
			return
		}
		var geo pgtype.UUID
		if body.HostSlug != "" {
			if g, err := h.service.store.read.GetSpaceBySlugKind(ctx, gen.GetSpaceBySlugKindParams{Slug: body.HostSlug, TypeSlug: "country"}); err == nil {
				geo = g
			}
		}
		if _, err := h.service.store.write.CreateBankForWiki(ctx, gen.CreateBankForWikiParams{
			Name: name, Slug: body.Slug, GeoSpaceID: geo, CountryCode: strPtr(body.CountryCode),
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}

	case "visa_route":
		dest, err := h.service.store.read.GetSpaceBySlugKind(ctx, gen.GetSpaceBySlugKindParams{Slug: body.DestinationSlug, TypeSlug: "country"})
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "destination_not_found", "Destination country not found")
			return
		}
		if _, err := h.service.store.write.CreateVisaRouteForWiki(ctx, gen.CreateVisaRouteForWikiParams{
			Name: name, Slug: body.Slug, DestinationSpaceID: dest,
			VisaCategory: body.VisaCategory, GovSourceUrl: strPtr(body.GovSourceURL),
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}

	case "scholarship":
		if _, err := h.service.store.write.CreateScholarshipForWiki(ctx, gen.CreateScholarshipForWikiParams{
			Title: name, Slug: body.Slug,
			SourceUrl: strPtr(body.SourceURL), Country: strPtr(body.Country),
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "kind_not_creatable", "This entity kind cannot be created here")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": out})
}

func (h *Handlers) failEntityCreate(w http.ResponseWriter, typeSlug string, err error) {
	var pge *pgconn.PgError
	if errors.As(err, &pge) && pge.Code == "23505" {
		errpkg.WriteError(w, http.StatusConflict, "slug_conflict", "An entity with this slug already exists")
		return
	}
	slog.Error("wiki: create entity failed", "type", typeSlug, "error", err)
	errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create entity")
}

func isAlpha(s string) bool {
	for _, c := range s {
		if c < 'a' || c > 'z' {
			if c < 'A' || c > 'Z' {
				return false
			}
		}
	}
	return true
}

func strPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

// entitySlug derives a URL-safe slug from a title — lowercases, keeps
// a-z0-9, collapses other runs into single hyphens. Used when the wiki
// create form omits a slug (mirrors the admin auto-generate behavior).
func entitySlug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastDash := true // keeps the slug from starting with a hyphen
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// HandleListProductProviders serves GET /products/{slug}/providers —
// the providers repeater source for the review form + product CTAs.
func (h *Handlers) HandleListProductProviders(w http.ResponseWriter, r *http.Request) {
	p, err := h.service.store.read.GetProductBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "product_not_found", "Product not found")
		return
	}
	rows, err := h.service.store.read.ListProductProviders(r.Context(), p.ID)
	if err != nil {
		slog.Error("wiki: list providers failed", "slug", p.Slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list providers")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, pp := range rows {
		items = append(items, map[string]any{
			"provider_id": pp.ProviderID.String(), "provider_slug": pp.ProviderSlug,
			"name": pp.Name, "price_from": pp.PriceFrom, "offer_url": pp.OfferUrl,
			"has_deal": pp.HasDeal,
		})
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"items": items}})
}

// HandleSetProductProvider serves POST+DELETE
// /admin/wiki/products/{slug}/providers/{provider_id} — the N:M link.
func (h *Handlers) HandleSetProductProvider(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider_id")
	slug := chi.URLParam(r, "slug")

	unlink := r.Method == http.MethodDelete
	var price, offer string
	var pos int32
	if !unlink {
		var body struct {
			PriceFrom string `json:"price_from"`
			OfferURL  string `json:"offer_url"`
			Position  int32  `json:"position"`
		}
		// EOF = empty body (bare link) — valid; other decode errors 400.
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		price, offer, pos = body.PriceFrom, body.OfferURL, body.Position
	}
	if err := h.service.SetProductProvider(r.Context(), slug, providerID, price, offer, pos, unlink); err != nil {
		writeErr(w, err, "set_failed", "Failed to set product provider")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleListPickerSpaces serves GET /wiki/pickers/spaces?type={type_slug} —
// university/country option lists for the entity create forms.
func (h *Handlers) HandleListPickerSpaces(w http.ResponseWriter, r *http.Request) {
	typeSlug := r.URL.Query().Get("type")
	if typeSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "type_required", "type query param required")
		return
	}
	rows, err := h.service.store.read.ListSpacesByTypeKind(r.Context(), gen.ListSpacesByTypeKindParams{
		TypeSlug: typeSlug, Q: r.URL.Query().Get("q"),
	})
	if err != nil {
		slog.Error("wiki: picker spaces failed", "type", typeSlug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list spaces")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID.String(), "slug": row.Slug, "name": row.Name})
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"items": items}})
}
