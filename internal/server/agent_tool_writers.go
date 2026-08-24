// Package server — agent_tool_writers.go wires the per-tool service
// Create/Update/Delete methods into the agentToolsRegistry's generic
// CreateTool/UpdateTool/DeleteTool/AssignGeo dispatch.
//
// Each per-tool service exposes its own typed input and DTO structs. To
// avoid a 112-closure explosion, this file uses reflection to marshal the
// generic map[string]any payload into the appropriate input struct, call
// the service, and marshal the returned DTO back to map[string]any.
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
	scholarship         any
	visaRoute           any
	housingGuide        any
	attestationGuide    any
	arrivalChecklist    any
	credentialPathway   any
	languageTestReq     any
	taxObligation       any
	communityOrg        any
	wikiPage            any
	calendarEntry       any

	// V2 tool tables
	scamReport          any
	planningGroup       any
	visaTimeline        any
	processingStat      any
	digestEdition       any
	mutualAid           any
	embassy             any
	appointmentAlert    any
	credentialProfession any
	cvFormatter         any
	languageTest        any
	mentorship          any
	remittanceRate      any
	remittanceProvider  any
	readinessAssessment any
	taxTreatyPair       any
	travelProvider      any
}

// buildToolWriters creates a toolWriter for each tool in toolCatalog that
// has a non-nil service pointer in the set.
func buildToolWriters(svc toolServiceSet) map[string]*toolWriter {
	serviceByTool := map[string]any{
		"scholarship":         svc.scholarship,
		"visa_route":          svc.visaRoute,
		"housing_guide":       svc.housingGuide,
		"attestation_guide":   svc.attestationGuide,
		"arrival_checklist":   svc.arrivalChecklist,
		"credential_pathway":  svc.credentialPathway,
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
		"arrival_checklist":   {update: "UpdateGuide"},
		"language_test_requirement": {create: "CreateRequirement", update: "UpdateRequirement"},
		"wiki_page":           {update: "UpdatePage"},
		"calendar_entry":      {update: "UpdateEvent"}, // if different; Update is common
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

func defaultMethodNames(names methodNames) methodNames {
	if names.create == "" {
		names.create = "Create"
	}
	if names.update == "" {
		names.update = "Update"
	}
	if names.delete == "" {
		names.delete = "Delete"
	}
	return names
}

func reflectCreateFunc(svc reflect.Value, methodName string) func(context.Context, pgtype.UUID, map[string]any) (map[string]any, error) {
	name := methodName
	if name == "" {
		name = "Create"
	}
	return func(ctx context.Context, tenantID pgtype.UUID, payload map[string]any) (map[string]any, error) {
		return reflectCallCreateOrUpdate(svc, name, ctx, tenantID, "", payload)
	}
}

func reflectUpdateFunc(svc reflect.Value, methodName string) func(context.Context, pgtype.UUID, string, map[string]any) (map[string]any, error) {
	name := methodName
	if name == "" {
		name = "Update"
	}
	return func(ctx context.Context, tenantID pgtype.UUID, id string, payload map[string]any) (map[string]any, error) {
		return reflectCallCreateOrUpdate(svc, name, ctx, tenantID, id, payload)
	}
}

func reflectDeleteFunc(svc reflect.Value, methodName string) func(context.Context, pgtype.UUID, string) error {
	name := methodName
	if name == "" {
		name = "Delete"
	}
	return func(ctx context.Context, tenantID pgtype.UUID, id string) error {
		m := svc.MethodByName(name)
		if !m.IsValid() {
			return fmt.Errorf("delete method %q not found", name)
		}
		mt := m.Type()
		if mt.NumIn() != 3 || mt.NumOut() != 1 {
			return fmt.Errorf("delete method %q has unsupported signature", name)
		}
		var idUUID pgtype.UUID
		if err := idUUID.Scan(id); err != nil {
			return fmt.Errorf("invalid tool_id: %w", err)
		}
		out := m.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(tenantID)})
		if !out[0].IsNil() {
			return out[0].Interface().(error)
		}
		return nil
	}
}

func reflectCallCreateOrUpdate(svc reflect.Value, methodName string, ctx context.Context, tenantID pgtype.UUID, id string, payload map[string]any) (map[string]any, error) {
	m := svc.MethodByName(methodName)
	if !m.IsValid() {
		return nil, fmt.Errorf("method %q not found", methodName)
	}
	mt := m.Type()

	isUpdate := false
	switch mt.NumIn() {
	case 3:
		isUpdate = false
	case 4:
		isUpdate = true
	default:
		return nil, fmt.Errorf("method %q has %d inputs", methodName, mt.NumIn())
	}
	if mt.NumOut() != 2 {
		return nil, fmt.Errorf("method %q has %d outputs", methodName, mt.NumOut())
	}

	inputIdx := 2
	if isUpdate {
		inputIdx = 3
	}

	inputType := mt.In(inputIdx)
	inputPtr := reflect.New(inputType)
	if err := mapToStruct(payload, inputPtr.Interface()); err != nil {
		return nil, fmt.Errorf("invalid payload for %s: %w", methodName, err)
	}
	inputVal := inputPtr.Elem()

	args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(tenantID)}
	if isUpdate {
		var idUUID pgtype.UUID
		if err := idUUID.Scan(id); err != nil {
			return nil, fmt.Errorf("invalid tool_id: %w", err)
		}
		args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(idUUID), reflect.ValueOf(tenantID)}
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
