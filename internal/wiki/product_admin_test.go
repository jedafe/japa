package wiki

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// productTestService builds a Service on a real Postgres via opsTestStore —
// skipped without JC_DATABASE_URL.
func productTestService(t *testing.T) (*Service, *db.DB) {
	t.Helper()
	store, database := opsTestStore(t)
	return NewService(store, nil), database
}

// seedProviderDeal inserts an affiliate provider + an active coupon for it.
func seedProviderDeal(t *testing.T, ctx context.Context, database *db.DB, uniq string) (providerID, couponID pgtype.UUID) {
	t.Helper()
	providerID = mustIDF(t, ctx, database,
		`INSERT INTO affiliate_providers (id,slug,name,provider_type,is_active)
		 VALUES (gen_random_uuid(),$1,$2,'both',true) RETURNING id`,
		"pp-provider-"+uniq, "PP Provider "+uniq)
	couponID = mustIDF(t, ctx, database,
		`INSERT INTO coupons (id,slug,title,provider_id,status,coupon_type)
		 VALUES (gen_random_uuid(),$1,$2,$3,'active','external') RETURNING id`,
		"pp-deal-"+uniq, "PP Deal "+uniq, providerID)
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM coupons WHERE id = $1`, couponID)
		database.Writer.Exec(cctx, `DELETE FROM affiliate_providers WHERE id = $1`, providerID)
	})
	return providerID, couponID
}

// seedProductGroup inserts a field group + one text field bound to a product
// type slug via cfga(entity_class = slug).
func seedProductGroup(t *testing.T, ctx context.Context, database *db.DB, typeSlug, uniq string) (groupID, fieldID pgtype.UUID) {
	t.Helper()
	groupID = mustIDF(t, ctx, database,
		`INSERT INTO custom_field_groups (id,name,slug,is_system)
		 VALUES (gen_random_uuid(),$1,$2,true) RETURNING id`,
		"PP Group "+uniq, "pp_group_"+uniq)
	fieldID = mustIDF(t, ctx, database,
		`INSERT INTO custom_fields (id,group_id,key,label,field_type,position)
		 VALUES (gen_random_uuid(),$1,$2,$3,'text',0) RETURNING id`,
		groupID, "pp_field_"+uniq, "PP Field "+uniq)
	mustIDF(t, ctx, database,
		`INSERT INTO custom_field_group_assignments (id,group_id,entity_class,entity_id,position)
		 VALUES (gen_random_uuid(),$1,$2,NULL,0) RETURNING id`,
		groupID, typeSlug)
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM custom_field_group_assignments WHERE group_id = $1`, groupID)
		database.Writer.Exec(cctx, `DELETE FROM custom_fields WHERE group_id = $1`, groupID)
		database.Writer.Exec(cctx, `DELETE FROM custom_field_groups WHERE id = $1`, groupID)
	})
	return groupID, fieldID
}

func productTypeIDFromSlug(t *testing.T, ctx context.Context, database *db.DB, slug string) pgtype.UUID {
	t.Helper()
	return mustIDF(t, ctx, database, `SELECT id FROM product_types WHERE slug = $1`, slug)
}

func TestCreateProductWithLinks(t *testing.T) {
	svc, database := productTestService(t)
	ctx := t.Context()
	uniq := genUUID(t, ctx, database)[:8]

	providerID, couponID := seedProviderDeal(t, ctx, database, uniq)
	_, fieldID := seedProductGroup(t, ctx, database, "saas", uniq)
	typeID := productTypeIDFromSlug(t, ctx, database, "saas")

	text := "field value"
	slug, err := svc.CreateProduct(ctx, ProductInput{
		TypeSlug: "saas", Slug: "pp-prod-" + uniq, Name: "PP Product " + uniq,
		Providers: []ProductProviderInput{{ProviderID: providerID.String(), PriceFrom: "9.99", OfferURL: "https://example.com"}},
		DealIDs:   []string{couponID.String()},
		CustomFields: []ProductFieldValueInput{{
			FieldID: fieldID.String(), ValueText: &text,
		}},
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	var productID pgtype.UUID
	if err := database.Reader.QueryRow(ctx, `SELECT id FROM products WHERE slug = $1`, slug).Scan(&productID); err != nil {
		t.Fatalf("product row: %v", err)
	}
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM custom_field_values WHERE entity_type='product' AND entity_id = $1`, productID)
		database.Writer.Exec(cctx, `DELETE FROM product_deals WHERE product_id = $1`, productID)
		database.Writer.Exec(cctx, `DELETE FROM product_providers WHERE product_id = $1`, productID)
		database.Writer.Exec(cctx, `DELETE FROM products WHERE id = $1`, productID)
	})

	var gotProvider, gotCoupon, gotField pgtype.UUID
	if err := database.Reader.QueryRow(ctx, `SELECT provider_id FROM product_providers WHERE product_id = $1`, productID).Scan(&gotProvider); err != nil {
		t.Fatalf("provider link: %v", err)
	}
	if gotProvider != providerID {
		t.Fatalf("provider link = %v, want %v", gotProvider, providerID)
	}
	if err := database.Reader.QueryRow(ctx, `SELECT coupon_id FROM product_deals WHERE product_id = $1`, productID).Scan(&gotCoupon); err != nil {
		t.Fatalf("deal link: %v", err)
	}
	if gotCoupon != couponID {
		t.Fatalf("deal link = %v, want %v", gotCoupon, couponID)
	}
	if err := database.Reader.QueryRow(ctx,
		`SELECT field_id FROM custom_field_values WHERE entity_type='product' AND entity_id = $1`, productID).Scan(&gotField); err != nil {
		t.Fatalf("field value: %v", err)
	}
	if gotField != fieldID {
		t.Fatalf("field value = %v, want %v", gotField, fieldID)
	}
	if productID.Bytes == [16]byte{} || typeID != productTypeIDFromSlug(t, ctx, database, "saas") {
		t.Fatal("sanity: product/type ids")
	}
}

func TestUpdateProductReplaceLinks(t *testing.T) {
	svc, database := productTestService(t)
	ctx := t.Context()
	uniq := genUUID(t, ctx, database)[:8]

	provA, dealA := seedProviderDeal(t, ctx, database, uniq+"a")
	provB, dealB := seedProviderDeal(t, ctx, database, uniq+"b")
	_, fieldID := seedProductGroup(t, ctx, database, "saas", uniq)

	slug, err := svc.CreateProduct(ctx, ProductInput{
		TypeSlug: "saas", Slug: "pp-prod-" + uniq, Name: "PP Product " + uniq,
		Providers: []ProductProviderInput{{ProviderID: provA.String()}, {ProviderID: provB.String()}},
		DealIDs:   []string{dealA.String(), dealB.String()},
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	var productID pgtype.UUID
	if err := database.Reader.QueryRow(ctx, `SELECT id FROM products WHERE slug = $1`, slug).Scan(&productID); err != nil {
		t.Fatalf("product row: %v", err)
	}
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM custom_field_values WHERE entity_type='product' AND entity_id = $1`, productID)
		database.Writer.Exec(cctx, `DELETE FROM product_deals WHERE product_id = $1`, productID)
		database.Writer.Exec(cctx, `DELETE FROM product_providers WHERE product_id = $1`, productID)
		database.Writer.Exec(cctx, `DELETE FROM products WHERE id = $1`, productID)
	})

	// PATCH: keep only provB, clear all deals, set + then clear a field value.
	text := "v"
	if err := svc.UpdateProduct(ctx, slug, ProductInput{
		Providers:    []ProductProviderInput{{ProviderID: provB.String()}},
		DealIDs:      []string{},
		CustomFields: []ProductFieldValueInput{{FieldID: fieldID.String(), ValueText: &text}},
	}); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}

	var n int
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*) FROM product_providers WHERE product_id = $1`, productID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("provider count = %d err=%v, want 1", n, err)
	}
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*) FROM product_deals WHERE product_id = $1`, productID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("deal count = %d err=%v, want 0", n, err)
	}
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*) FROM custom_field_values WHERE entity_type='product' AND entity_id = $1`, productID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cfv count = %d err=%v, want 1", n, err)
	}

	if err := svc.UpdateProduct(ctx, slug, ProductInput{
		ClearFieldIDs: []string{fieldID.String()},
	}); err != nil {
		t.Fatalf("UpdateProduct clear: %v", err)
	}
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*) FROM custom_field_values WHERE entity_type='product' AND entity_id = $1`, productID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cfv after clear = %d err=%v, want 0", n, err)
	}
}

func TestCreateProductTypeWritesAssignments(t *testing.T) {
	svc, database := productTestService(t)
	ctx := t.Context()
	uniq := genUUID(t, ctx, database)[:8]
	slug := "pp-type-" + uniq

	err := svc.CreateProductType(ctx, slug, "PP Type "+uniq, "box", []ProductTypeCategory{
		{Slug: "overview", Label: "Overview"},
		{Slug: "specs", Label: "Specs"},
	}, nil)
	if err != nil {
		t.Fatalf("CreateProductType: %v", err)
	}
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx,
			`DELETE FROM custom_field_group_assignments WHERE entity_class = $1`, slug)
		database.Writer.Exec(cctx,
			`DELETE FROM comparison_type_field_groups WHERE type_id IN (SELECT id FROM comparison_type_definitions WHERE slug = $1)`, slug)
		database.Writer.Exec(cctx, `DELETE FROM comparison_type_definitions WHERE slug = $1`, slug)
		database.Writer.Exec(cctx,
			`DELETE FROM custom_field_groups WHERE slug LIKE 'wiki\_' || replace($1,'-','_') || '\_%' ESCAPE '\'`, slug)
		database.Writer.Exec(cctx, `DELETE FROM product_types WHERE slug = $1`, slug)
	})

	var n int
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*) FROM custom_field_group_assignments WHERE entity_class = $1 AND entity_id IS NULL`, slug).Scan(&n); err != nil {
		t.Fatalf("assignments: %v", err)
	}
	if n != 2 {
		t.Fatalf("assignments = %d, want 2", n)
	}
}

func TestUpdateProductTypeAppendsCategory(t *testing.T) {
	svc, database := productTestService(t)
	ctx := t.Context()
	uniq := genUUID(t, ctx, database)[:8]
	slug := "pp-type-" + uniq

	if err := svc.CreateProductType(ctx, slug, "PP Type "+uniq, "", nil, nil); err != nil {
		t.Fatalf("CreateProductType: %v", err)
	}
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx,
			`DELETE FROM custom_field_group_assignments WHERE entity_class = $1`, slug)
		database.Writer.Exec(cctx,
			`DELETE FROM comparison_type_field_groups WHERE type_id IN (SELECT id FROM comparison_type_definitions WHERE slug = $1)`, slug)
		database.Writer.Exec(cctx, `DELETE FROM comparison_type_definitions WHERE slug = $1`, slug)
		database.Writer.Exec(cctx,
			`DELETE FROM custom_field_groups WHERE slug LIKE 'wiki\_' || replace($1,'-','_') || '\_%' ESCAPE '\'`, slug)
		database.Writer.Exec(cctx, `DELETE FROM product_types WHERE slug = $1`, slug)
	})

	newLabel := "PP Type Renamed"
	enabled := false
	if err := svc.UpdateProductType(ctx, slug, ProductTypeInput{
		Label:     &newLabel,
		IsEnabled: &enabled,
		Categories: []ProductTypeCategory{
			{Slug: "pricing", Label: "Pricing"},
		},
	}); err != nil {
		t.Fatalf("UpdateProductType: %v", err)
	}

	var label string
	var isEnabled bool
	if err := database.Reader.QueryRow(ctx,
		`SELECT label, is_enabled FROM product_types WHERE slug = $1`, slug).Scan(&label, &isEnabled); err != nil {
		t.Fatalf("type row: %v", err)
	}
	if label != newLabel || isEnabled {
		t.Fatalf("type = (%q,%v), want (%q,false)", label, isEnabled, newLabel)
	}

	var n int
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*) FROM custom_field_group_assignments WHERE entity_class = $1 AND entity_id IS NULL`, slug).Scan(&n); err != nil || n != 2 {
		t.Fatalf("assignments = %d err=%v, want 2 (overview + pricing)", n, err)
	}
	// Pool row on the comparison type for the new category.
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*) FROM comparison_type_field_groups ctfg
		 JOIN comparison_type_definitions d ON d.id = ctfg.type_id
		 WHERE d.slug = $1`, slug).Scan(&n); err != nil || n != 2 {
		t.Fatalf("pool rows = %d err=%v, want 2", n, err)
	}
}

func TestDeleteProductTypeInUse(t *testing.T) {
	svc, database := productTestService(t)
	ctx := t.Context()
	uniq := genUUID(t, ctx, database)[:8]
	typeSlug := "pp-type-" + uniq

	if err := svc.CreateProductType(ctx, typeSlug, "PP Type "+uniq, "", nil, nil); err != nil {
		t.Fatalf("CreateProductType: %v", err)
	}
	prodSlug := "pp-prod-" + uniq
	if _, err := svc.CreateProduct(ctx, ProductInput{
		TypeSlug: typeSlug, Slug: prodSlug, Name: "PP Product " + uniq,
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM products WHERE slug = $1`, prodSlug)
		database.Writer.Exec(cctx,
			`DELETE FROM custom_field_group_assignments WHERE entity_class = $1`, typeSlug)
		database.Writer.Exec(cctx,
			`DELETE FROM comparison_type_field_groups WHERE type_id IN (SELECT id FROM comparison_type_definitions WHERE slug = $1)`, typeSlug)
		database.Writer.Exec(cctx, `DELETE FROM comparison_type_definitions WHERE slug = $1`, typeSlug)
		database.Writer.Exec(cctx,
			`DELETE FROM custom_field_groups WHERE slug LIKE 'wiki\_' || replace($1,'-','_') || '\_%' ESCAPE '\'`, typeSlug)
		database.Writer.Exec(cctx, `DELETE FROM product_types WHERE slug = $1`, typeSlug)
	})

	if err := svc.DeleteProductType(ctx, typeSlug); !errors.Is(err, errProductTypeInUse) {
		t.Fatalf("DeleteProductType = %v, want errProductTypeInUse", err)
	}
	if _, err := database.Writer.Exec(ctx, `DELETE FROM products WHERE slug = $1`, prodSlug); err != nil {
		t.Fatalf("delete product: %v", err)
	}
	if err := svc.DeleteProductType(ctx, typeSlug); err != nil {
		t.Fatalf("DeleteProductType after product removal: %v", err)
	}
}

func TestProductFieldSchema(t *testing.T) {
	svc, database := productTestService(t)
	ctx := t.Context()
	uniq := genUUID(t, ctx, database)[:8]
	typeSlug := "pp-schema-" + uniq
	_, fieldID := seedProductGroup(t, ctx, database, typeSlug, uniq)

	groups, err := svc.ProductFieldSchema(ctx, typeSlug)
	if err != nil {
		t.Fatalf("ProductFieldSchema: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if len(groups[0].Fields) != 1 || groups[0].Fields[0].ID != fieldID.String() {
		t.Fatalf("fields = %+v, want one field %s", groups[0].Fields, fieldID)
	}
}

func TestAdminListProductsContract(t *testing.T) {
	svc, database := productTestService(t)
	ctx := t.Context()
	uniq := genUUID(t, ctx, database)[:8]

	slug, err := svc.CreateProduct(ctx, ProductInput{
		TypeSlug: "saas", Slug: "pp-list-" + uniq, Name: "PP List " + uniq,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	var productID pgtype.UUID
	_ = database.Reader.QueryRow(ctx, `SELECT id FROM products WHERE slug = $1`, slug).Scan(&productID)
	t.Cleanup(func() {
		database.Writer.Exec(t.Context(), `DELETE FROM products WHERE id = $1`, productID)
	})

	rows, total, err := svc.AdminListProducts(ctx, "saas", uniq, "active", "name:asc", 25, 0)
	if err != nil {
		t.Fatalf("AdminListProducts: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].Slug != slug {
		t.Fatalf("list = %d rows total=%d, want 1 row %s", len(rows), total, slug)
	}

	// status=deleted hides the live row; after soft delete it appears.
	if _, total, err = svc.AdminListProducts(ctx, "saas", uniq, "deleted", "", 25, 0); err != nil || total != 0 {
		t.Fatalf("deleted filter before delete = %d err=%v, want 0", total, err)
	}
	if err := svc.DeleteProduct(ctx, slug); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
	if _, total, err = svc.AdminListProducts(ctx, "saas", uniq, "deleted", "", 25, 0); err != nil || total != 1 {
		t.Fatalf("deleted filter after delete = %d err=%v, want 1", total, err)
	}
	if err := svc.RestoreProduct(ctx, slug); err != nil {
		t.Fatalf("RestoreProduct: %v", err)
	}
	if _, total, err = svc.AdminListProducts(ctx, "saas", uniq, "active", "", 25, 0); err != nil || total != 1 {
		t.Fatalf("active filter after restore = %d err=%v, want 1", total, err)
	}
}

func TestSetProductDealUnlink(t *testing.T) {
	svc, database := productTestService(t)
	ctx := t.Context()
	uniq := genUUID(t, ctx, database)[:8]

	_, couponID := seedProviderDeal(t, ctx, database, uniq)
	slug, err := svc.CreateProduct(ctx, ProductInput{
		TypeSlug: "saas", Slug: "pp-deal-" + uniq, Name: "PP DealProd " + uniq,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	var productID pgtype.UUID
	_ = database.Reader.QueryRow(ctx, `SELECT id FROM products WHERE slug = $1`, slug).Scan(&productID)
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM product_deals WHERE product_id = $1`, productID)
		database.Writer.Exec(cctx, `DELETE FROM products WHERE id = $1`, productID)
	})

	if err := svc.SetProductDeal(ctx, slug, couponID.String(), 3, false); err != nil {
		t.Fatalf("SetProductDeal link: %v", err)
	}
	var n int
	var pos int32
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*), max(position) FROM product_deals WHERE product_id = $1`, productID).Scan(&n, &pos); err != nil || n != 1 || pos != 3 {
		t.Fatalf("deal link = %d pos=%d err=%v, want 1 pos 3", n, pos, err)
	}
	if err := svc.SetProductDeal(ctx, slug, couponID.String(), 0, true); err != nil {
		t.Fatalf("SetProductDeal unlink: %v", err)
	}
	if err := database.Reader.QueryRow(ctx,
		`SELECT count(*) FROM product_deals WHERE product_id = $1`, productID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("deal link after unlink = %d err=%v, want 0", n, err)
	}
}

