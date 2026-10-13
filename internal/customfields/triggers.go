package customfields

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// TriggerDispatcher dispatches form submission triggers. It is the
// customfields package's implementation of worker.FormTriggerProcessor.
//
// The dispatcher holds optional references to the platform services that
// each trigger type needs (rbac for assign_user_group, tickets for
// create_ticket, notifications for notify_admin). Missing dependencies are
// logged and skipped — a form with a trigger whose backing service is not
// wired does not block submission processing.
//
// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md.
type TriggerDispatcher struct {
	service *Service

	// rbacAssigner assigns a user to a user group (assign_user_group trigger).
	// Matches the rbac.Store.AddUserToGroup signature.
	rbacAssigner func(ctx context.Context, userID, groupID pgtype.UUID) error

	// ticketCreator creates a ticket (create_ticket trigger).
	ticketCreator func(ctx context.Context, subject, description string, submitterUserID *pgtype.UUID) error

	// notifyAdmin sends an admin notification (notify_admin trigger).
	notifyAdmin func(ctx context.Context, title, body, actionURL string) error

	// emailSender sends an email (send_email trigger). Returns the send error.
	emailSender func(ctx context.Context, to, subject, body string) error

	// httpClient is used for send_webhook triggers. Defaults to a 10s-timeout
	// client when nil.
	httpClient *http.Client
}

// NewTriggerDispatcher creates a dispatcher backed by the given service. The
// optional service hooks (rbacAssigner, ticketCreator, notifyAdmin,
// emailSender) are wired via Set* methods after construction.
func NewTriggerDispatcher(service *Service) *TriggerDispatcher {
	return &TriggerDispatcher{
		service:    service,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// SetRBACAssigner wires the function used to add a user to a group for the
// assign_user_group trigger type.
func (d *TriggerDispatcher) SetRBACAssigner(f func(ctx context.Context, userID, groupID pgtype.UUID) error) {
	d.rbacAssigner = f
}

// SetTicketCreator wires the function used to create a ticket for the
// create_ticket trigger type.
func (d *TriggerDispatcher) SetTicketCreator(f func(ctx context.Context, subject, description string, submitterUserID *pgtype.UUID) error) {
	d.ticketCreator = f
}

// SetNotifyAdmin wires the function used to send an admin notification for
// the notify_admin trigger type.
func (d *TriggerDispatcher) SetNotifyAdmin(f func(ctx context.Context, title, body, actionURL string) error) {
	d.notifyAdmin = f
}

// SetEmailSender wires the function used to send email for the send_email
// trigger type.
func (d *TriggerDispatcher) SetEmailSender(f func(ctx context.Context, to, subject, body string) error) {
	d.emailSender = f
}

// FireSubmissionTriggers implements worker.FormTriggerProcessor. It loads the
// triggers configured on the form and dispatches each by trigger_type. Errors
// from individual triggers are logged but do not abort the batch — a
// misconfigured send_webhook should not prevent an assign_user_group from
// firing. Errors that prevent loading the triggers themselves are returned.
//
// Idempotency: each trigger type's backing service is responsible for
// deduplicating on submissionID (e.g. AddUserToGroup is idempotent via the
// unique membership constraint).
func (d *TriggerDispatcher) FireSubmissionTriggers(
	ctx context.Context,
	submissionID, formID pgtype.UUID,
	submitterUserID *pgtype.UUID,
	fieldValues []byte,
) error {
	triggers, err := d.service.read.ListFormSubmissionTriggersByForm(ctx, formID)
	if err != nil {
		return fmt.Errorf("listing form submission triggers: %w", err)
	}

	if len(triggers) == 0 {
		return nil
	}

	// Parse field_values once for triggers that need to read submission data.
	var values map[string]any
	if len(fieldValues) > 0 {
		_ = json.Unmarshal(fieldValues, &values)
	}

	for _, trig := range triggers {
		if err := d.dispatchOne(ctx, trig, submissionID, submitterUserID, values); err != nil {
			slog.Warn("customfields: form trigger failed",
				"submission_id", submissionID.String(),
				"trigger_type", trig.TriggerType,
				"trigger_id", trig.ID.String(),
				"error", err)
		}
	}

	return nil
}

// dispatchOne fires a single trigger by type.
func (d *TriggerDispatcher) dispatchOne(
	ctx context.Context,
	trig gen.FormSubmissionTrigger,
	submissionID pgtype.UUID,
	submitterUserID *pgtype.UUID,
	values map[string]any,
) error {
	var cfg map[string]any
	if len(trig.Config) > 0 {
		if err := json.Unmarshal(trig.Config, &cfg); err != nil {
			return fmt.Errorf("decoding trigger config: %w", err)
		}
	}

	switch trig.TriggerType {
	case "assign_user_group":
		return d.fireAssignUserGroup(ctx, cfg, submitterUserID)

	case "send_email":
		return d.fireSendEmail(ctx, cfg, values)

	case "send_webhook":
		return d.fireSendWebhook(ctx, cfg, submissionID, values)

	case "create_ticket":
		return d.fireCreateTicket(ctx, cfg, submitterUserID, values)

	case "notify_admin":
		return d.fireNotifyAdmin(ctx, cfg, submissionID, values)

	default:
		slog.Warn("customfields: unknown form trigger type, skipping",
			"trigger_type", trig.TriggerType, "trigger_id", trig.ID.String())
		return nil
	}
}

// fireAssignUserGroup adds the submitter to the configured user group. No-op
// for anonymous submissions (no submitterUserID).
func (d *TriggerDispatcher) fireAssignUserGroup(ctx context.Context, cfg map[string]any, submitterUserID *pgtype.UUID) error {
	if submitterUserID == nil {
		slog.Info("customfields: assign_user_group trigger skipped — anonymous submission")
		return nil
	}

	if d.rbacAssigner == nil {
		slog.Warn("customfields: assign_user_group trigger — no rbac assigner wired, skipping")
		return nil
	}

	groupIDStr, _ := cfg["user_group_id"].(string)
	if groupIDStr == "" {
		return errors.New("assign_user_group: missing user_group_id in config")
	}

	var groupID pgtype.UUID
	if err := groupID.Scan(groupIDStr); err != nil {
		return fmt.Errorf("assign_user_group: invalid user_group_id: %w", err)
	}

	return d.rbacAssigner(ctx, *submitterUserID, groupID)
}

// fireSendEmail sends an email to the configured address. Supports templating
// of subject/body with field values via {{field_key}} placeholders.
func (d *TriggerDispatcher) fireSendEmail(ctx context.Context, cfg, values map[string]any) error {
	if d.emailSender == nil {
		slog.Warn("customfields: send_email trigger — no email sender wired, skipping")
		return nil
	}

	to, _ := cfg["to"].(string)
	subject, _ := cfg["subject"].(string)
	body, _ := cfg["body"].(string)

	if to == "" {
		return errors.New("send_email: missing 'to' in config")
	}

	return d.emailSender(ctx, to, expandTemplate(subject, values), expandTemplate(body, values))
}

// fireSendWebhook POSTs the submission payload to the configured URL.
// The URL scheme allowlist (http/https) is enforced at trigger create/update
// time (validateTriggerConfig) and re-checked here so rows predating the
// gate can never fire a non-http(s) sink (CF14).
func (d *TriggerDispatcher) fireSendWebhook(ctx context.Context, cfg map[string]any, submissionID pgtype.UUID, values map[string]any) error {
	url, _ := cfg["url"].(string)
	if url == "" {
		return errors.New("send_webhook: missing 'url' in config")
	}
	if !validWebhookURL(url) {
		return errors.New("send_webhook: 'url' must be an http(s) URL")
	}

	payload := map[string]any{
		"submission_id": submissionID.String(),
		"field_values":  values,
		"sent_at":       time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("send_webhook: encoding payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("send_webhook: building request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if secret, _ := cfg["secret"].(string); secret != "" {
		req.Header.Set("X-Form-Webhook-Secret", secret)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send_webhook: POST %s: %w", url, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("send_webhook: POST %s returned %d", url, resp.StatusCode)
	}

	return nil
}

// fireCreateTicket creates a ticket from the submission.
func (d *TriggerDispatcher) fireCreateTicket(ctx context.Context, cfg map[string]any, submitterUserID *pgtype.UUID, values map[string]any) error {
	if d.ticketCreator == nil {
		slog.Warn("customfields: create_ticket trigger — no ticket creator wired, skipping")
		return nil
	}

	subject, _ := cfg["subject"].(string)
	if subject == "" {
		subject = "Form submission"
	}

	description, _ := cfg["description"].(string)
	if description == "" {
		// Default to a JSON dump of the field values so the ticket body is useful.
		if b, err := json.MarshalIndent(values, "", "  "); err == nil {
			description = string(b)
		}
	}

	return d.ticketCreator(ctx, expandTemplate(subject, values), expandTemplate(description, values), submitterUserID)
}

// fireNotifyAdmin sends an admin notification about the submission.
func (d *TriggerDispatcher) fireNotifyAdmin(ctx context.Context, cfg map[string]any, submissionID pgtype.UUID, values map[string]any) error {
	if d.notifyAdmin == nil {
		slog.Warn("customfields: notify_admin trigger — no notifyAdmin wired, skipping")
		return nil
	}

	title, _ := cfg["title"].(string)
	if title == "" {
		title = "New form submission"
	}

	body, _ := cfg["body"].(string)
	if body == "" {
		body = fmt.Sprintf("Submission %s received.", submissionID.String())
	}

	actionURL, _ := cfg["action_url"].(string)

	return d.notifyAdmin(ctx, expandTemplate(title, values), expandTemplate(body, values), actionURL)
}

// validWebhookURL reports whether raw is an http(s) URL with a host — the
// CF14 scheme allowlist shared by the create/update validator and the
// fire-time re-check.
func validWebhookURL(raw string) bool {
	parsed, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// expandTemplate replaces {{key}} placeholders in s with the corresponding
// value from values. Unknown keys are left as-is.
func expandTemplate(s string, values map[string]any) string {
	for k, v := range values {
		placeholder := "{{" + k + "}}"
		s = strings.ReplaceAll(s, placeholder, fmt.Sprintf("%v", v))
	}

	return s
}
