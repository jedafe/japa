// ACTION_TYPES lists every action_type value written to audit_logs across
// the backend. The control panel audit-log filter dropdown is populated from
// this list, so it MUST stay in sync with the string literals passed to the
// INSERT INTO audit_logs calls in:
//   - internal/auth/handlers.go          (auth.login_failed, user.password_reset)
//   - internal/impersonation/service.go  (impersonate.start, impersonate.end)
//   - internal/control/handlers.go       (tenant.delete, tenant.suspend, tenant.unsuspend)
//   - internal/visatimeline/store.go     (visa_timeline.implausible_sequence)
// If you add a new audit-log call, add the action_type here too.
// NOTE: 'impersonation.start'/'impersonation.end' are kept for compatibility
// with rows written by an earlier version of the impersonation service; the
// current code writes 'impersonate.start'/'impersonate.end'.
export const ACTION_TYPES = [
	'auth.login_failed',
	'user.password_reset',
	'impersonate.start',
	'impersonate.end',
	'impersonation.start',
	'impersonation.end',
	'tenant.delete',
	'tenant.suspend',
	'tenant.unsuspend',
	'visa_timeline.implausible_sequence'
] as const;

export type AuditActionType = (typeof ACTION_TYPES)[number];

// TARGET_TYPES lists every target_type value written to audit_logs. The
// control panel target-type filter dropdown is populated from this list.
// Sources:
//   - internal/auth/handlers.go          ('user')
//   - internal/impersonation/service.go  ('user')
//   - internal/control/handlers.go       ('tenant')
//   - internal/visatimeline/store.go     ('visa_timeline_milestone')
export const TARGET_TYPES = [
	'user',
	'tenant',
	'visa_timeline_milestone'
] as const;

export type AuditTargetType = (typeof TARGET_TYPES)[number];
