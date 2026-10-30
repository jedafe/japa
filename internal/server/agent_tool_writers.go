// Package server — agent_tool_writers.go wires the per-tool service
// Create/Update/Delete methods into the agentToolsRegistry's generic
// CreateTool/UpdateTool/DeleteTool/AssignGeo dispatch.
// // Each per-tool service exposes its own typed input and DTO structs. To
// avoid a 112-closure explosion, this file uses reflection to marshal the
// generic map[string]any payload into the appropriate input struct, call
// the service, and marshal the returned DTO back to map[string]any.
// // Many services have non-standard method names (CreateGuide, UpdateGuide,
// CreatePathway, etc.) or require an extra user/owner/caller UUID argument.
// The reflection helpers detect the input type from the method signature
// and supply the calling userID when the method takes an extra pgtype.UUID
// parameter.
// // Ref: docs/superpowers/specs/2026-08-24-dedicated-ai-tool-crud.md
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// toolServiceSet holds the service instances that back the tool catalog.
// It is constructed in server.go with the concrete per-tool *Service
// pointers already wired at startup.
type toolServiceSet struct {
	// Original (Phase 9 + baseline) tool tables
	scholarship       any
	visaRoute         any
	housingGuide      any
	attestationGuide  any
	arrivalChecklist  any
	credentialPathway any
	languageTestReq   any
	taxObligation     any
	communityOrg      any
	wikiPage          any
	calendarEntry     any

	// V2 tool tables
	scamReport           any
	planningGroup        any
	visaTimeline         any
	processingStat       any
	digestEdition        any
	mutualAid            any
	embassy              any
	appointmentAlert     any
	credentialProfession any
	cvFormatter          any
	languageTest         any
	mentorship           any
	remittanceRate       any
	remittanceProvider   any
	readinessAssessment  any
	taxTreatyPair        any
	travelProvider       any

	// Store handles for read-modify-write merges where the getter lives on
	// the store but the update lives on the service.
	remittanceProviderStore any
	travelProviderStore     any

	// Calculator + reference-data tool tables (§107, §108, §109, §110).
	crsCalculator any
	pofCalculator any
	sopGenerator  any
	ecaGuide      any
}

// buildToolWriters creates a toolWriter for each tool in toolCatalog that
// has a non-nil service pointer in the set.
func buildToolWriters(svc toolServiceSet) map[string]*toolWriter {
	serviceByTool := map[string]any{
		"scholarship":               svc.scholarship,
		"visa_route":                svc.visaRoute,
		"housing_guide":             svc.housingGuide,
		"attestation_guide":         svc.attestationGuide,
		"arrival_checklist":         svc.arrivalChecklist,
		"credential_pathway":        svc.credentialPathway,
		"language_test_requirement": svc.languageTestReq,
		"tax_obligation_guide":      svc.taxObligation,
		"community_organisation":    svc.communityOrg,
		"wiki_page":                 svc.wikiPage,
		"calendar_entry":            svc.calendarEntry,
		"scam_report":               svc.scamReport,
		"planning_group":            svc.planningGroup,
		"visa_timeline":             svc.visaTimeline,
		"processing_time":           svc.processingStat,
		"digest_edition":            svc.digestEdition,
		"mutual_aid":                svc.mutualAid,
		"embassy":                   svc.embassy,
		"appointment_alert":         svc.appointmentAlert,
		"credential_profession":     svc.credentialProfession,
		"cv_formatter":              svc.cvFormatter,
		"language_test":             svc.languageTest,
		"mentorship_connection":     svc.mentorship,
		"remittance_rate":           svc.remittanceRate,
		"remittance_provider":       svc.remittanceProvider,
		"readiness_assessment":      svc.readinessAssessment,
		"tax_treaty_pair":           svc.taxTreatyPair,
		"travel_provider":           svc.travelProvider,
		// Calculator + reference-data tools (§107, §108, §109, §110).
		"crs_ruleset":     svc.crsCalculator,
		"crs_draw":        svc.crsCalculator,
		"pof_requirement": svc.pofCalculator,
		"sop_template":    svc.sopGenerator,
		"sop_sample":      svc.sopGenerator,
		"sop_field":       svc.sopGenerator,
		"eca_guide":       svc.ecaGuide,
		"eca_provider":    svc.ecaGuide,
	}

	// Method name overrides for tools whose service methods don't follow the
	// default "Create"/"Update"/"Delete" convention. An empty field falls
	// back to the default name; a name that doesn't resolve to a real method
	// leaves the op unregistered (nil closure → graceful "not supported"
	// error at call time and exclusion from WritableToolOps).
	methodOverrides := map[string]methodNames{
		"arrival_checklist":         {create: "CreateGuide", update: "UpdateGuide", delete: "DeleteGuide"},
		"credential_pathway":        {create: "CreatePathway", update: "UpdatePathway", delete: "DeletePathway"},
		"credential_profession":     {create: "CreateProfession", update: "UpdateProfession", delete: "DeleteProfession"},
		"language_test":             {create: "CreateLanguageTest", update: "UpdateLanguageTest", delete: "DeleteLanguageTest"},
		"language_test_requirement": {create: "CreateRequirement", update: "UpdateRequirement", delete: "DeleteRequirement"},
		"processing_time":           {create: "CreateProcessingTime", update: "UpdateProcessingTime", delete: "DeleteProcessingTime"},
		"readiness_assessment":      {create: "CreateAssessment", update: "UpdateAssessment", delete: "DeleteAssessment"},
		"remittance_rate":           {create: "CreateRate", update: "UpdateRate", delete: "DeleteRate"},
		"tax_obligation_guide":      {create: "CreateGuide", update: "UpdateGuide", delete: "DeleteGuide"},
		"visa_timeline":             {create: "Create", update: "UpdateOwn", delete: "DeleteOwn"},
		"planning_group":            {create: "Create", update: "UpdateOwn", delete: "DeleteOwn"},
		"mutual_aid":                {create: "Create", update: "UpdateOwn", delete: "DeleteOwn"},
		"travel_provider":           {create: "CreateProvider", update: "UpdateProvider", delete: "DeleteProvider", mergeGetter: "GetProviderByID"},
		"remittance_provider":       {create: "CreateProvider", update: "UpdateProvider", delete: "DeleteProvider", mergeGetter: "GetProviderByID"},
		"embassy":                   {create: "CreateEmbassy", update: "UpdateEmbassy"},
		"appointment_alert":         {create: "CreateAlert", update: "UpdateAlert", delete: "DeleteAlert"},
		"calendar_entry":            {create: "CreateEntry", update: "UpdateEntry", delete: "AdminSoftDeleteEntry", mergeGetter: "GetEntry"},
		"scam_report":               {create: "CreateReport", update: "UpdateReport", mergeGetter: "GetReportForUpdate"},
		"cv_formatter":              {create: "CreateStandard", update: "UpdateStandard", delete: "DeleteStandard", mergeGetter: "GetStandardByDest", mergeIDKey: "destination_space_id"},
		"tax_treaty_pair":           {create: "CreateOrUpdateTreaty", update: "UpdateTreaty", delete: "DeleteTreaty"},
		"digest_edition":            {create: "CreateEdition", update: "UpdateEdition", delete: "DeleteEdition"},
		"scholarship":               {create: "AdminCreateScholarship", update: "UpdateScholarship", delete: "DeleteScholarship"},
		"community_organisation":    {create: "CreateOrg", update: "UpdateOrg", delete: "DeleteOrg"},
		"crs_ruleset":               {create: "CreateRuleset", update: "UpdateRuleset", delete: "DeleteRuleset"},
		"crs_draw":                  {create: "CreateDraw", update: "UpdateDraw", delete: "DeleteDraw"},
		"pof_requirement":           {create: "CreateRequirement", update: "UpdateRequirement", delete: "DeleteRequirement"},
		"sop_template":              {create: "CreateTemplate", update: "UpdateTemplate", delete: "DeleteTemplate"},
		"sop_sample":                {create: "CreateSample", update: "UpdateSample", delete: "DeleteSample"},
		"sop_field":                 {create: "CreateField", update: "UpdateField", delete: "DeleteField"},
		"eca_guide":                 {create: "CreateGuide", update: "UpdateGuide", delete: "DeleteGuide"},
		"eca_provider":              {create: "CreateProvider", update: "UpdateProvider", delete: "DeleteProvider"},
	}

	// Services whose merge getter lives on the store rather than the
	// service the update is dispatched to.
	getterSvcByTool := map[string]any{
		"remittance_provider": svc.remittanceProviderStore,
		"travel_provider":     svc.travelProviderStore,
	}

	writers := make(map[string]*toolWriter)

	for toolType, service := range serviceByTool {
		if service == nil {
			continue
		}

		names := methodOverrides[toolType]
		v := reflect.ValueOf(service)

		updateName := resolveWriteMethod(v, names.update, "Update")

		// assign_geo only resolves when the update input can actually carry
		// a geo field — otherwise the tool would no-op silently.
		assignGeoName := ""
		if updateName != "" && methodLastArgHasGeoField(v, updateName) {
			assignGeoName = updateName
		}

		updateFunc := reflectUpdateFunc(v, updateName, names.updateUserArg)
		assignGeoFunc := reflectUpdateFunc(v, assignGeoName, names.updateUserArg)

		// Full-row update inputs need read-modify-write. When a mergeGetter
		// is declared but cannot be resolved, fail closed: better no update
		// op than one that zero-fills fields the caller didn't send.
		if names.mergeGetter != "" {
			getterSvc := v
			if gs, ok := getterSvcByTool[toolType]; ok && gs != nil {
				getterSvc = reflect.ValueOf(gs)
			}

			merged := newMergeUpdateFunc(getterSvc, names.mergeGetter, names.mergeIDKey, v, updateName, names.updateUserArg)
			if merged == nil {
				updateFunc, assignGeoFunc = nil, nil
			} else {
				updateFunc = merged

				if assignGeoName != "" {
					assignGeoFunc = merged
				}
			}
		}

		writers[toolType] = &toolWriter{
			create:    reflectCreateFunc(v, resolveWriteMethod(v, names.create, "Create")),
			update:    updateFunc,
			delete:    reflectDeleteFunc(v, resolveDeleteMethod(v, names.delete)),
			assignGeo: assignGeoFunc,
		}
	}

	// Wiki pages use store methods with explicit positional arguments
	// rather than a single DTO. Wire bespoke closures: CreatePage for
	// create, UpdatePage for the page-flag subset of update (body edits go
	// through the revision workflow per §80 — not a direct write), and
	// DeleteWikiPage for soft delete.
	if svc, ok := serviceByTool["wiki_page"]; ok && svc != nil {
		v := reflect.ValueOf(svc)
		if m := v.MethodByName("CreatePage"); m.IsValid() {
			writers["wiki_page"] = &toolWriter{
				create:    newWikiCreateFunc(m),
				update:    newWikiUpdateFunc(v.MethodByName("UpdatePage")),
				delete:    reflectDeleteFunc(v, "DeleteWikiPage"),
				assignGeo: nil,
			}
		}
	}

	// Mentorship connections require multiple non-struct arguments.
	if svc, ok := serviceByTool["mentorship_connection"]; ok && svc != nil {
		m := reflect.ValueOf(svc).MethodByName("CreateConnection")
		if m.IsValid() {
			writers["mentorship_connection"] = &toolWriter{
				create:    newMentorshipCreateFunc(m),
				update:    nil,
				delete:    nil,
				assignGeo: nil,
			}
		}
	}

	return writers
}

func newWikiCreateFunc(m reflect.Value) func(context.Context, pgtype.UUID, map[string]any) (map[string]any, error) {
	return func(ctx context.Context, userID pgtype.UUID, payload map[string]any) (map[string]any, error) {
		spaceID, err := getPgUUID(payload, "space_id")
		if err != nil {
			return nil, fmt.Errorf("invalid space_id: %w", err)
		}

		slug, _ := payload["slug"].(string)
		if slug == "" {
			return nil, errors.New("slug is required")
		}

		title, _ := payload["title"].(string)
		if title == "" {
			return nil, errors.New("title is required")
		}

		category, _ := payload["category"].(string)
		if category == "" {
			category = "general"
		}

		out := m.Call([]reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(spaceID),
			reflect.ValueOf(userID),
			reflect.ValueOf(slug),
			reflect.ValueOf(title),
			reflect.ValueOf(category),
		})
		if !out[1].IsNil() {
			return nil, out[1].Interface().(error)
		}

		return structToMap(out[0].Interface())
	}
}

// newWikiUpdateFunc adapts wiki.Store.UpdatePage(ctx, pageID, isLocked,
// isFeatured, isActive *bool, dataEntityType *string, position *int32) to the
// generic update closure. The payload keys are the page's flag fields;
// absent keys are passed as nil (unchanged). Body edits are not accepted —
// §80 routes content changes through the revision queue.
func newWikiUpdateFunc(m reflect.Value) func(context.Context, pgtype.UUID, string, map[string]any) (map[string]any, error) {
	return func(ctx context.Context, userID pgtype.UUID, id string, payload map[string]any) (map[string]any, error) {
		if !m.IsValid() {
			return nil, errors.New("wiki: UpdatePage method not found")
		}

		var pageID pgtype.UUID
		if err := pageID.Scan(id); err != nil {
			return nil, fmt.Errorf("invalid tool_id: %w", err)
		}

		optBool := func(key string) *bool {
			v, ok := payload[key]
			if !ok || v == nil {
				return nil
			}

			b, ok := v.(bool)
			if !ok {
				return nil
			}

			return &b
		}

		var position *int32

		if v, ok := payload["position"]; ok && v != nil {
			if f, ok := v.(float64); ok {
				p := int32(f)
				position = &p
			}
		}

		out := m.Call([]reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(pageID),
			reflect.ValueOf(optBool("is_locked")),
			reflect.ValueOf(optBool("is_featured")),
			reflect.ValueOf(optBool("is_active")),
			reflect.ValueOf(getOptionalString(payload, "data_entity_type")),
			reflect.ValueOf(position),
		})
		if !out[0].IsNil() {
			return nil, out[0].Interface().(error)
		}

		return map[string]any{"id": id, "updated": true}, nil
	}
}

func getPgUUID(payload map[string]any, key string) (pgtype.UUID, error) {
	var u pgtype.UUID

	v, ok := payload[key]
	if !ok {
		return u, fmt.Errorf("%s is required", key)
	}

	s, ok := v.(string)
	if !ok {
		return u, fmt.Errorf("%s must be a string", key)
	}

	if err := u.Scan(s); err != nil {
		return u, err
	}

	return u, nil
}

func getOptionalString(payload map[string]any, key string) *string {
	v, ok := payload[key]
	if !ok {
		return nil
	}

	if v == nil {
		return nil
	}

	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}

	return &s
}

func newMentorshipCreateFunc(m reflect.Value) func(context.Context, pgtype.UUID, map[string]any) (map[string]any, error) {
	return func(ctx context.Context, userID pgtype.UUID, payload map[string]any) (map[string]any, error) {
		mentorID, err := getPgUUID(payload, "mentor_id")
		if err != nil {
			return nil, fmt.Errorf("invalid mentor_id: %w", err)
		}

		menteeID, err := getPgUUID(payload, "mentee_id")
		if err != nil {
			return nil, fmt.Errorf("invalid mentee_id: %w", err)
		}

		destSpaceID, err := getPgUUID(payload, "destination_space_id")
		if err != nil {
			return nil, fmt.Errorf("invalid destination_space_id: %w", err)
		}

		goals := getOptionalString(payload, "goals")

		out := m.Call([]reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(mentorID),
			reflect.ValueOf(menteeID),
			reflect.ValueOf(goals),
			reflect.ValueOf(destSpaceID),
		})
		if !out[1].IsNil() {
			return nil, out[1].Interface().(error)
		}

		return structToMap(out[0].Interface())
	}
}

type methodNames struct {
	create string
	update string
	delete string
	// updateUserArg marks an update3 method whose second argument is the
	// calling user's UUID, not the record id — Update(ctx, userID, input)
	// with the record id carried inside the input struct. Reflection can't
	// tell a userID from a record ID (both are pgtype.UUID), so the shape
	// must be declared explicitly or the call silently mis-binds.
	updateUserArg bool
	// mergeGetter names a read method used for read-modify-write updates on
	// services whose update input is a full row/params struct rather than a
	// partial-fields DTO (UpdateReport, UpdateEntry, UpdateProvider,
	// UpdateStandard). Without the merge, a partial payload zero-fills
	// every field it omits.
	mergeGetter string
	// mergeIDKey names the payload key that supplies the getter's id
	// argument when the record isn't keyed by its own primary key — e.g.
	// cv_formatter standards are upsert-keyed by destination_space_id.
	// Empty means the getter is called with the record id.
	mergeIDKey string
}

// resolveWriteMethod returns the name of the create/update method to call:
// the explicit override when set, otherwise the default. Returns "" when no
// candidate resolves to a method with a supported signature — the caller
// leaves the op nil so no dead tool is registered.
func resolveWriteMethod(svc reflect.Value, preferred, fallback string) string {
	name := preferred
	if name == "" {
		name = fallback
	}

	if name != "" && validWriteSignature(svc, name) {
		return name
	}

	return ""
}

// validWriteSignature reports whether the named method matches one of the
// create/update shapes reflectCallCreateOrUpdate can dispatch:
//
//	Create: (ctx, input) | (ctx, userID, input)
//	Update: (ctx, input) | (ctx, id, input) | (ctx, id, userID, input)
//
// The trailing input argument must be a struct (or pointer to struct) or a
// map — anything else (e.g. a status string or a bare pgtype.UUID as the
// last parameter) cannot be populated from the payload map.
func validWriteSignature(svc reflect.Value, name string) bool {
	m := svc.MethodByName(name)
	if !m.IsValid() {
		return false
	}

	mt := m.Type()

	nIn, nOut := mt.NumIn(), mt.NumOut()
	if nOut != 1 && nOut != 2 {
		return false
	}

	if nIn == 0 || !isContextArg(mt.In(0)) {
		return false
	}

	uuidType := reflect.TypeOf(pgtype.UUID{})

	var arityOK bool

	switch {
	case isCreateMethodName(name):
		// (ctx, input) or (ctx, userID, input)
		arityOK = nIn == 2 || (nIn == 3 && mt.In(1) == uuidType)
	case isUpdateMethodName(name):
		// (ctx, input) | (ctx, id|userID, input) | (ctx, id, userID, input)
		// A 3-arg update's middle UUID is assumed to be the record id unless
		// methodNames.updateUserArg declares it the caller — reflection
		// cannot distinguish the two.
		arityOK = nIn == 2 ||
			(nIn == 3 && mt.In(1) == uuidType) ||
			(nIn == 4 && mt.In(1) == uuidType && mt.In(2) == uuidType)
	default:
		return false
	}

	if !arityOK || !isPayloadInputType(mt.In(nIn-1)) {
		return false
	}

	return true
}

// isContextArg reports whether t implements context.Context.
func isContextArg(t reflect.Type) bool {
	return t.Implements(reflect.TypeOf((*context.Context)(nil)).Elem())
}

// isPayloadInputType reports whether t can be populated by mapToStruct —
// a struct or *struct DTO (excluding scalar wrappers such as pgtype.UUID),
// or a map type that receives the payload keys directly.
func isPayloadInputType(t reflect.Type) bool {
	uuidType := reflect.TypeOf(pgtype.UUID{})
	if t == uuidType {
		return false
	}

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t.Kind() == reflect.Struct || t.Kind() == reflect.Map
}

// methodLastArgHasGeoField reports whether the resolved update method's
// trailing input type contains a geo-scope field (any *_space_id style
// field, matching json tags or field names, up to a shallow nesting depth).
// assign_geo forwards its payload to the update method, so without such a
// field the op would silently do nothing.
func methodLastArgHasGeoField(svc reflect.Value, name string) bool {
	m := svc.MethodByName(name)
	if !m.IsValid() {
		return false
	}

	return typeHasGeoField(m.Type().In(m.Type().NumIn()-1), 0)
}

func typeHasGeoField(t reflect.Type, depth int) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct || depth > 3 {
		return false
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.ToLower(f.Name)
		tag := strings.ToLower(strings.Split(f.Tag.Get("json"), ",")[0])

		if geoFieldName(name) || geoFieldName(tag) {
			return true
		}

		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		if (f.Anonymous || ft.Kind() == reflect.Struct) && !strings.HasPrefix(ft.String(), "pgtype.") {
			if typeHasGeoField(ft, depth+1) {
				return true
			}
		}
	}

	return false
}

// geoFieldName matches geo-scope field identifiers: destination_space_id,
// origin_space_id, issuing_space_id, city_space_id, space_id, and similar —
// anything containing "space" and ending in "id".
func geoFieldName(n string) bool {
	return strings.Contains(n, "space") && strings.HasSuffix(n, "id")
}

// validDeleteSignature reports whether the named method matches a dispatch
// shape in reflectDeleteFunc: (ctx, id) or (ctx, id, userID), returning
// nothing or a single error.
func validDeleteSignature(svc reflect.Value, name string) bool {
	m := svc.MethodByName(name)
	if !m.IsValid() {
		return false
	}

	mt := m.Type()
	if mt.NumOut() > 1 || !isContextArg(mt.In(0)) {
		return false
	}

	uuidType := reflect.TypeOf(pgtype.UUID{})

	switch mt.NumIn() {
	case 2: // (ctx, id)
		return mt.In(1) == uuidType
	case 3: // (ctx, id, userID)
		return mt.In(1) == uuidType && mt.In(2) == uuidType
	default:
		return false
	}
}

// resolveDeleteMethod returns the name of the delete method to call. An
// explicit override must resolve as-is; when empty, "Delete" and then the
// delete-like fallbacks are tried in order.
func resolveDeleteMethod(svc reflect.Value, preferred string) string {
	if preferred != "" {
		if validDeleteSignature(svc, preferred) {
			return preferred
		}

		return ""
	}

	for _, name := range []string{"Delete", "Deactivate", "Archive", "SoftDelete", "DeleteOwn", "DeleteOrg"} {
		if validDeleteSignature(svc, name) {
			return name
		}
	}

	return ""
}

func reflectCreateFunc(svc reflect.Value, methodName string) func(context.Context, pgtype.UUID, map[string]any) (map[string]any, error) {
	name := methodName
	if name == "" {
		return nil
	}

	return func(ctx context.Context, userID pgtype.UUID, payload map[string]any) (map[string]any, error) {
		return reflectCallCreateOrUpdate(svc, name, ctx, userID, "", payload, false)
	}
}

func reflectUpdateFunc(svc reflect.Value, methodName string, userArg bool) func(context.Context, pgtype.UUID, string, map[string]any) (map[string]any, error) {
	name := methodName
	if name == "" {
		return nil
	}

	return func(ctx context.Context, userID pgtype.UUID, id string, payload map[string]any) (map[string]any, error) {
		return reflectCallCreateOrUpdate(svc, name, ctx, userID, id, payload, userArg)
	}
}

// newMergeUpdateFunc builds a read-modify-write update closure for services
// whose update method takes a full row or sqlc params struct — every column
// is written unconditionally, so a partial payload would zero-fill the
// fields it omits. The getter's current row is marshalled to a map, the
// caller's payload is overlaid, and the merged map is unmarshalled into the
// update input. Returns nil when the getter or update method cannot be
// resolved — the caller fails closed and leaves the op unregistered.
//
// Getter shapes supported: (ctx, id) or (ctx, id, userID). When mergeIDKey
// is set, the getter's id argument is read from that payload key instead of
// the record id (upsert-keyed records like cv_standards). The getter may
// live on a different service value than the updater (e.g. remittance's
// GetProviderByID is on the store while UpdateProvider is on the service).
func newMergeUpdateFunc(getterSvc reflect.Value, getterName, mergeIDKey string, updateSvc reflect.Value, updateName string, userArg bool) func(context.Context, pgtype.UUID, string, map[string]any) (map[string]any, error) {
	g := getterSvc.MethodByName(getterName)
	if updateName == "" || !g.IsValid() || !updateSvc.MethodByName(updateName).IsValid() {
		return nil
	}

	gt := g.Type()
	if gt.NumIn() != 2 && gt.NumIn() != 3 {
		return nil
	}

	return func(ctx context.Context, userID pgtype.UUID, id string, payload map[string]any) (map[string]any, error) {
		key := id

		if mergeIDKey != "" {
			s, _ := payload[mergeIDKey].(string)
			if s == "" {
				return nil, fmt.Errorf("%s is required for this record's update", mergeIDKey)
			}

			key = s
		}

		var keyUUID pgtype.UUID
		if err := keyUUID.Scan(key); err != nil {
			return nil, fmt.Errorf("invalid lookup id: %w", err)
		}

		args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(keyUUID)}
		if gt.NumIn() == 3 {
			args = append(args, reflect.ValueOf(userID))
		}

		out := g.Call(args)
		if len(out) == 2 && !out[1].IsNil() {
			return nil, out[1].Interface().(error)
		}

		base, err := structToMap(out[0].Interface())
		if err != nil {
			return nil, fmt.Errorf("marshal current row for merge: %w", err)
		}

		for k, v := range payload {
			base[k] = v
		}

		return reflectCallCreateOrUpdate(updateSvc, updateName, ctx, userID, id, base, userArg)
	}
}

func reflectDeleteFunc(svc reflect.Value, methodName string) func(context.Context, pgtype.UUID, string) error {
	name := methodName
	if name == "" {
		return nil
	}

	return func(ctx context.Context, userID pgtype.UUID, id string) error {
		m := svc.MethodByName(name)
		if !m.IsValid() {
			return fmt.Errorf("delete method %q not found", name)
		}

		mt := m.Type()

		var args []reflect.Value

		switch mt.NumIn() {
		case 2:
			// (ctx, id)
			var idUUID pgtype.UUID
			if err := idUUID.Scan(id); err != nil {
				return fmt.Errorf("invalid tool_id: %w", err)
			}

			args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID)}
		case 3:
			// (ctx, id, userID)
			var idUUID pgtype.UUID
			if err := idUUID.Scan(id); err != nil {
				return fmt.Errorf("invalid tool_id: %w", err)
			}

			args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(userID)}
		default:
			return fmt.Errorf("delete method %q has %d inputs", name, mt.NumIn())
		}

		out := m.Call(args)
		if len(out) == 0 || out[0].IsNil() {
			return nil
		}

		return out[0].Interface().(error)
	}
}

func reflectCallCreateOrUpdate(svc reflect.Value, methodName string, ctx context.Context, userID pgtype.UUID, id string, payload map[string]any, updateUserArg bool) (map[string]any, error) {
	m := svc.MethodByName(methodName)
	if !m.IsValid() {
		return nil, fmt.Errorf("method %q not found", methodName)
	}

	mt := m.Type()

	isCreate := isCreateMethodName(methodName)
	isUpdate := isUpdateMethodName(methodName)

	nIn, nOut := mt.NumIn(), mt.NumOut()
	if nOut != 1 && nOut != 2 {
		return nil, fmt.Errorf("method %q has %d outputs", methodName, nOut)
	}

	// Supported write-method signatures (single-platform):
	// Create: (ctx, input)
	// Create: (ctx, userID, input)
	// Update: (ctx, input) -- input contains id
	// Update: (ctx, id, input)
	// Update: (ctx, userID, input) -- updateUserArg, input contains id
	// Update: (ctx, id, userID, input)
	var (
		create2     = isCreate && nIn == 2
		create3     = isCreate && nIn == 3
		update2     = isUpdate && nIn == 2
		update3     = isUpdate && nIn == 3 && !updateUserArg
		update3User = isUpdate && nIn == 3 && updateUserArg
		update4     = isUpdate && nIn == 4
	)

	if !create2 && !create3 && !update2 && !update3 && !update3User && !update4 {
		return nil, fmt.Errorf("method %q has %d inputs and %d outputs", methodName, nIn, nOut)
	}

	// Merge and id into the payload so 2-argument methods that
	// take a single gen/DTO input receive them without bespoke plumbing.
	mergedPayload := make(map[string]any, len(payload)+2)
	for k, v := range payload {
		mergedPayload[k] = v
	}

	if id != "" {
		mergedPayload["id"] = id
	}

	var idUUID pgtype.UUID
	if update3 || update4 {
		if err := idUUID.Scan(id); err != nil {
			return nil, fmt.Errorf("invalid tool_id: %w", err)
		}
	}

	inputIdx := nIn - 1

	inputType := mt.In(inputIdx)

	inputPtr := reflect.New(inputType)
	if err := mapToStruct(mergedPayload, inputPtr.Interface()); err != nil {
		return nil, fmt.Errorf("invalid payload for %s: %w", methodName, err)
	}

	inputVal := inputPtr.Elem()

	var args []reflect.Value

	switch {
	case create2, update2:
		args = []reflect.Value{reflect.ValueOf(ctx)}
	case update3:
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID)}
	case create3, update3User:
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(userID)}
	case update4:
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(userID)}
	}

	args = append(args, inputVal)

	out := m.Call(args)

	switch nOut {
	case 1:
		// Methods such as UpdateStandard only return an error.
		if !out[0].IsNil() {
			return nil, out[0].Interface().(error)
		}

		return nil, nil
	case 2:
		if !out[1].IsNil() {
			if err, ok := out[1].Interface().(error); ok {
				return nil, err
			}

			return nil, fmt.Errorf("method %q returned non-error second value", methodName)
		}

		return structToMap(out[0].Interface())
	}

	return nil, nil
}

func isCreateMethodName(name string) bool {
	return name == "Create" || strings.HasPrefix(name, "Create") || name == "AdminCreateScholarship"
}

func isUpdateMethodName(name string) bool {
	return name == "Update" || strings.HasPrefix(name, "Update")
}

func mapToStruct(m map[string]any, v any) error {
	// First pass: try the original snake_case map. This works for DTOs and
	// gen structs that already have json tags.
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(b, v); err != nil {
		return err
	}

	// Second pass: convert the keys to PascalCase (with acronym fixes) and
	// unmarshal again. This fills fields on structs that have no json tags
	// and therefore expect the field name as the JSON key.
	pascal := make(map[string]any, len(m))
	for k, val := range m {
		pascal[snakeToPascal(k)] = val
	}

	b2, err := json.Marshal(pascal)
	if err != nil {
		return err
	}

	return json.Unmarshal(b2, v)
}

func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")

	var out strings.Builder

	for _, p := range parts {
		if p == "" {
			continue
		}

		out.WriteString(strings.ToUpper(p[:1]))

		if len(p) > 1 {
			out.WriteString(p[1:])
		}
	}

	res := out.String()
	res = strings.ReplaceAll(res, "Id", "ID")
	res = strings.ReplaceAll(res, "Url", "URL")
	res = strings.ReplaceAll(res, "Usd", "USD")

	return res
}

func structToMap(v any) (map[string]any, error) {
	// Some store Create* methods return a bare pgtype.UUID. Convert it to a
	// simple record map so the generic writer contract stays uniform.
	if u, ok := v.(pgtype.UUID); ok {
		return map[string]any{"id": u.String()}, nil
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}

	return m, nil
}
