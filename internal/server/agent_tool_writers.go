// Package server — agent_tool_writers.go wires the per-tool service
// Create/Update/Delete methods into the agentToolsRegistry's generic
// CreateTool/UpdateTool/DeleteTool/AssignGeo dispatch.
//
// Each per-tool service exposes its own typed input and DTO structs. To
// avoid a 112-closure explosion, this file uses reflection to marshal the
// generic map[string]any payload into the appropriate input struct, call
// the service, and marshal the returned DTO back to map[string]any.
//
// Many services have non-standard method names (CreateGuide, UpdateGuide,
// CreatePathway, etc.) or require an extra user/owner/caller UUID argument.
// The reflection helpers detect the input type from the method signature
// and supply the calling userID when the method takes an extra pgtype.UUID
// parameter.
//
// Ref: docs/superpowers/specs/2026-08-24-dedicated-ai-tool-crud.md
package server

import (
	"context"
	"encoding/json"
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
	}

	// Method name overrides for tools whose service methods don't follow the
	// default "Create"/"Update"/"Delete" convention.
	methodOverrides := map[string]methodNames{
		"arrival_checklist":         {create: "CreateGuide", update: "UpdateGuide"},
		"credential_pathway":        {create: "CreatePathway", update: "UpdatePathway"},
		"credential_profession":     {create: "CreateProfession", update: "UpdateProfession", delete: "DeleteProfession"},
		"language_test":             {create: "CreateLanguageTest", update: "UpdateLanguageTest", delete: "DeleteLanguageTest"},
		"language_test_requirement": {create: "CreateRequirement"},
		"processing_time":           {create: "CreateProcessingTime", update: "UpdateProcessingTime", delete: "DeleteProcessingTime"},
		"readiness_assessment":      {create: "CreateAssessment", update: "UpdateAssessment", delete: "DeleteAssessment"},
		"remittance_rate":           {create: "CreateRate", update: "UpdateRate", delete: "DeleteRate"},
		"tax_obligation_guide":      {create: "CreateGuide", update: "UpdateGuide"},
		"visa_timeline":             {create: "Create", update: "UpdateOwn", delete: "DeleteOwn"},
		"planning_group":            {create: "Create", update: "UpdateOwn", delete: "DeleteOwn"},
		"mutual_aid":                {create: "Create", update: "UpdateOwn", delete: "DeleteOwn"},
		"travel_provider":           {create: "CreateProvider", update: "UpdateProvider"},
		"remittance_provider":       {create: "CreateProvider", update: "UpdateProvider"},
		"embassy":                   {create: "CreateEmbassy", update: "UpdateEmbassy"},
		"appointment_alert":         {create: "CreateAlert", update: "UpdateAlert", delete: "DeleteAlert"},
		"calendar_entry":            {create: "CreateEntry", update: "UpdateEntry"},
		"scam_report":               {create: "CreateReport", update: "UpdateReport"},
		"cv_formatter":              {create: "CreateStandard", update: "UpdateStandard"},
		"tax_treaty_pair":           {create: "CreateOrUpdateTreaty"},
		"digest_edition":            {create: "CreateEdition"},
		"scholarship":               {create: "AdminCreateScholarship"},
		"community_organisation":    {create: "CreateOrg"},
	}

	writers := make(map[string]*toolWriter)
	for toolType, service := range serviceByTool {
		if service == nil {
			continue
		}
		names := methodOverrides[toolType]
		v := reflect.ValueOf(service)
		writers[toolType] = &toolWriter{
			create:    reflectCreateFunc(v, names.create),
			update:    reflectUpdateFunc(v, names.update),
			delete:    reflectDeleteFunc(v, names.delete),
			assignGeo: reflectUpdateFunc(v, names.update),
		}
	}

	// Wiki pages use a store method with explicit positional arguments
	// rather than a single DTO. Wire a bespoke create closure.
	if svc, ok := serviceByTool["wiki_page"]; ok && svc != nil {
		m := reflect.ValueOf(svc).MethodByName("CreatePage")
		if m.IsValid() {
			writers["wiki_page"] = &toolWriter{
				create:    newWikiCreateFunc(m),
				update:    nil,
				delete:    nil,
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

func newWikiCreateFunc(m reflect.Value) func(context.Context, pgtype.UUID, pgtype.UUID, map[string]any) (map[string]any, error) {
	return func(ctx context.Context, tenantID, userID pgtype.UUID, payload map[string]any) (map[string]any, error) {
		spaceID, err := getPgUUID(payload, "space_id")
		if err != nil {
			return nil, fmt.Errorf("invalid space_id: %w", err)
		}
		slug, _ := payload["slug"].(string)
		if slug == "" {
			return nil, fmt.Errorf("slug is required")
		}
		title, _ := payload["title"].(string)
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		category, _ := payload["category"].(string)
		if category == "" {
			category = "general"
		}

		out := m.Call([]reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(tenantID),
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

func newMentorshipCreateFunc(m reflect.Value) func(context.Context, pgtype.UUID, pgtype.UUID, map[string]any) (map[string]any, error) {
	return func(ctx context.Context, tenantID, userID pgtype.UUID, payload map[string]any) (map[string]any, error) {
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
			reflect.ValueOf(tenantID),
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
}

func reflectCreateFunc(svc reflect.Value, methodName string) func(context.Context, pgtype.UUID, pgtype.UUID, map[string]any) (map[string]any, error) {
	name := methodName
	if name == "" {
		name = "Create"
	}
	return func(ctx context.Context, tenantID, userID pgtype.UUID, payload map[string]any) (map[string]any, error) {
		return reflectCallCreateOrUpdate(svc, name, ctx, tenantID, userID, "", payload)
	}
}

func reflectUpdateFunc(svc reflect.Value, methodName string) func(context.Context, pgtype.UUID, pgtype.UUID, string, map[string]any) (map[string]any, error) {
	name := methodName
	if name == "" {
		name = "Update"
	}
	return func(ctx context.Context, tenantID, userID pgtype.UUID, id string, payload map[string]any) (map[string]any, error) {
		return reflectCallCreateOrUpdate(svc, name, ctx, tenantID, userID, id, payload)
	}
}

func reflectDeleteFunc(svc reflect.Value, methodName string) func(context.Context, pgtype.UUID, pgtype.UUID, string) error {
	name := methodName
	if name == "" {
		name = "Delete"
	}
	return func(ctx context.Context, tenantID, userID pgtype.UUID, id string) error {
		m := svc.MethodByName(name)
		if !m.IsValid() {
			// Try common delete-like fallbacks.
			for _, alt := range []string{"Deactivate", "Archive", "SoftDelete", "DeleteOwn", "DeleteOrg"} {
				m = svc.MethodByName(alt)
				if m.IsValid() {
					name = alt
					break
				}
			}
		}
		if !m.IsValid() {
			return fmt.Errorf("delete method %q not found", name)
		}
		mt := m.Type()
		var args []reflect.Value
		switch mt.NumIn() {
		case 3:
			// (ctx, id, tenantID)
			var idUUID pgtype.UUID
			if err := idUUID.Scan(id); err != nil {
				return fmt.Errorf("invalid tool_id: %w", err)
			}
			args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(tenantID)}
		case 4:
			// (ctx, id, tenantID, userID)
			var idUUID pgtype.UUID
			if err := idUUID.Scan(id); err != nil {
				return fmt.Errorf("invalid tool_id: %w", err)
			}
			args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(tenantID), reflect.ValueOf(userID)}
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

func reflectCallCreateOrUpdate(svc reflect.Value, methodName string, ctx context.Context, tenantID, userID pgtype.UUID, id string, payload map[string]any) (map[string]any, error) {
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

	// Supported write-method signatures:
	//   Create: (ctx, input)
	//   Create: (ctx, tenantID, input)
	//   Create: (ctx, tenantID, userID, input)
	//   Update: (ctx, input) -- input contains id + tenant_id
	//   Update: (ctx, id, tenantID, input)
	//   Update: (ctx, id, tenantID, userID, input)
	var (
		create2 = isCreate && nIn == 2
		create3 = isCreate && nIn == 3
		create4 = isCreate && nIn == 4
		update2 = isUpdate && nIn == 2
		update4 = isUpdate && nIn == 4
		update5 = isUpdate && nIn == 5
	)
	if !create2 && !create3 && !create4 && !update2 && !update4 && !update5 {
		return nil, fmt.Errorf("method %q has %d inputs and %d outputs", methodName, nIn, nOut)
	}

	// Merge tenant_id and id into the payload so 2-argument methods that
	// take a single gen/DTO input receive them without bespoke plumbing.
	mergedPayload := make(map[string]any, len(payload)+2)
	for k, v := range payload {
		mergedPayload[k] = v
	}
	mergedPayload["tenant_id"] = tenantID.String()
	if id != "" {
		mergedPayload["id"] = id
	}

	var idUUID pgtype.UUID
	if update4 || update5 {
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
	case create3:
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(tenantID)}
	case create4:
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(tenantID), reflect.ValueOf(userID)}
	case update4:
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(tenantID)}
	case update5:
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(tenantID), reflect.ValueOf(userID)}
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
