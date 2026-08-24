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

	"github.com/jackc/pgx/v5/pgtype"
)

// toolServiceSet holds the service instances that back the tool catalog.
// It is constructed in server.go with the concrete per-tool *Service
// pointers already wired at startup.
type toolServiceSet struct {
	// Original (Phase 9 + baseline) tool tables
	scholarship          any
	visaRoute            any
	housingGuide         any
	attestationGuide     any
	arrivalChecklist     any
	credentialPathway    any
	languageTestReq      any
	taxObligation        any
	communityOrg         any
	wikiPage             any
	calendarEntry        any

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
		"scholarship":                 svc.scholarship,
		"visa_route":                  svc.visaRoute,
		"housing_guide":               svc.housingGuide,
		"attestation_guide":           svc.attestationGuide,
		"arrival_checklist":           svc.arrivalChecklist,
		"credential_pathway":          svc.credentialPathway,
		"language_test_requirement":   svc.languageTestReq,
		"tax_obligation_guide":        svc.taxObligation,
		"community_organisation":      svc.communityOrg,
		"wiki_page":                   svc.wikiPage,
		"calendar_entry":              svc.calendarEntry,
		"scam_report":                 svc.scamReport,
		"planning_group":              svc.planningGroup,
		"visa_timeline":               svc.visaTimeline,
		"processing_time":             svc.processingStat,
		"digest_edition":              svc.digestEdition,
		"mutual_aid":                  svc.mutualAid,
		"embassy":                     svc.embassy,
		"appointment_alert":           svc.appointmentAlert,
		"credential_profession":       svc.credentialProfession,
		"cv_formatter":                svc.cvFormatter,
		"language_test":               svc.languageTest,
		"mentorship_connection":       svc.mentorship,
		"remittance_rate":             svc.remittanceRate,
		"remittance_provider":         svc.remittanceProvider,
		"readiness_assessment":        svc.readinessAssessment,
		"tax_treaty_pair":             svc.taxTreatyPair,
		"travel_provider":             svc.travelProvider,
	}

	// Method name overrides for tools whose service methods don't follow the
	// default "Create"/"Update"/"Delete" convention.
	methodOverrides := map[string]methodNames{
		"arrival_checklist":         {create: "CreateGuide", update: "UpdateGuide", delete: "DeleteGuide"},
		"attestation_guide":         {create: "CreatePathway"},
		"credential_pathway":        {create: "CreatePathway", update: "UpdatePathway"},
		"language_test_requirement": {create: "CreateRequirement"},
		"tax_obligation_guide":      {create: "CreateGuide", update: "UpdateGuide"},
		"visa_timeline":             {create: "CreateTimeline", update: "UpdateTimeline"},
		"digest_edition":            {create: "CreateEdition", update: "UpdateEdition"},
		"planning_group":            {create: "Create", update: "UpdateOwn", delete: "DeleteOwn"},
		"scam_report":               {update: "Update"},
		"calendar_entry":            {update: "Update"},
		"mutual_aid":                {create: "CreatePost", update: "UpdatePost", delete: "DeletePost"},
		"travel_provider":           {create: "CreateProvider", update: "UpdateProvider"},
		"remittance_provider":       {create: "CreateProvider", update: "UpdateProvider"},
		"remittance_rate":           {create: "CreateRate", update: "UpdateRate"},
		"readiness_assessment":      {create: "CreateAssessment", update: "UpdateAssessment"},
		"mentorship_connection":     {create: "CreateConnection", update: "UpdateConnection"},
		"appointment_alert":         {create: "CreateAlert", update: "UpdateAlert"},
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
	return writers
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

	// Determine method shape:
	//   Create: 3 inputs (ctx, tenantID, input)
	//   Create: 4 inputs (ctx, tenantID, ownerID/userID, input)
	//   Update: 4 inputs (ctx, id, tenantID, input)
	//   Update: 5 inputs (ctx, id, tenantID, userID, input)
	create3 := mt.NumIn() == 3 && mt.NumOut() == 2
	create4 := mt.NumIn() == 4 && methodName == "Create" && mt.NumOut() == 2
	update4 := mt.NumIn() == 4 && mt.NumOut() == 2
	update5 := mt.NumIn() == 5 && mt.NumOut() == 2

	if !create3 && !create4 && !update4 && !update5 {
		return nil, fmt.Errorf("method %q has %d inputs and %d outputs", methodName, mt.NumIn(), mt.NumOut())
	}

	var idUUID pgtype.UUID
	if update4 || update5 {
		if err := idUUID.Scan(id); err != nil {
			return nil, fmt.Errorf("invalid tool_id: %w", err)
		}
	}

	inputIdx := 2
	if create4 {
		inputIdx = 3
	} else if update4 || update5 {
		inputIdx = mt.NumIn() - 1
	}

	inputType := mt.In(inputIdx)
	inputPtr := reflect.New(inputType)
	if err := mapToStruct(payload, inputPtr.Interface()); err != nil {
		return nil, fmt.Errorf("invalid payload for %s: %w", methodName, err)
	}
	inputVal := inputPtr.Elem()

	var args []reflect.Value
	if create3 {
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(tenantID)}
	} else if create4 {
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(tenantID), reflect.ValueOf(userID)}
	} else if update4 {
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(tenantID)}
	} else if update5 {
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(tenantID), reflect.ValueOf(userID)}
	}
	args = append(args, inputVal)

	out := m.Call(args)
	if !out[1].IsNil() {
		if err, ok := out[1].Interface().(error); ok {
			return nil, err
		}
		return nil, fmt.Errorf("method %q returned non-error second value", methodName)
	}
	return structToMap(out[0].Interface())
}

func mapToStruct(m map[string]any, v any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func structToMap(v any) (map[string]any, error) {
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
