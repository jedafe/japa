package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// FormTriggerProcessor dispatches form submission triggers for a given
// submission. The customfields service implements this interface; the worker
// calls it from HandleFormFireSubmissionTriggers.
//
// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md.
type FormTriggerProcessor interface {
	// FireSubmissionTriggers loads the triggers configured on the form that
	// received the submission and dispatches each by trigger_type
	// (assign_user_group / send_email / send_webhook / create_ticket /
	// notify_admin). It must be idempotent on submissionID — duplicate
	// deliveries are acceptable, duplicate side effects (e.g. double group
	// assignment) are not.
	FireSubmissionTriggers(ctx context.Context, submissionID, tenantID, formID pgtype.UUID, submitterUserID *pgtype.UUID, fieldValues []byte) error
}

var formTriggerProcessor FormTriggerProcessor

// RegisterFormTriggerProcessor registers the customfields service as the
// processor for form:fire_submission_triggers tasks.
func RegisterFormTriggerProcessor(p FormTriggerProcessor) {
	formTriggerProcessor = p
}
