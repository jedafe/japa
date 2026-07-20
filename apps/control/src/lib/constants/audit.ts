export const ACTION_TYPES = ['create', 'update', 'delete', 'login', 'logout'] as const;

export type AuditActionType = (typeof ACTION_TYPES)[number];
