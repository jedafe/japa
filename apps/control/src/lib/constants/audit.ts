/**
 * actionColorClass returns a Tailwind badge class for an audit action_type.
 * It matches by prefix/suffix so namespaced actions like `gdpr.account_deleted`
 * are classified correctly. Used by the control panel
 * audit-log action column renderer.
 */
export function actionColorClass(action: string): string {
	if (!action) return 'bg-accent text-accent-foreground';

	if (action === 'delete' || action.endsWith('.delete') || action.endsWith('_deleted')) {
		return 'bg-destructive/10 text-destructive';
	}
	if (action === 'create' || action.endsWith('.create')) {
		return 'bg-emerald-500/10 text-emerald-600';
	}
	if (action === 'update' || action.endsWith('.update')) {
		return 'bg-blue-500/10 text-blue-600';
	}
	if (action.endsWith('.moderate')) {
		return 'bg-violet-500/10 text-violet-600';
	}
	if (action.startsWith('impersonate') || action.startsWith('impersonation')) {
		return 'bg-blue-500/10 text-blue-600';
	}
	if (
		action.startsWith('auth.') ||
		action.includes('login_failed') ||
		action.includes('.password_reset') ||
		action.endsWith('.failed') ||
		action.endsWith('.suspend') ||
		action.endsWith('.unsuspend') ||
		action.endsWith('.retention_wait') ||
		action.includes('.implausible')
	) {
		return 'bg-amber-500/10 text-amber-600';
	}
	if (action.startsWith('gdpr.')) {
		return 'bg-rose-500/10 text-rose-600';
	}

	return 'bg-accent text-accent-foreground';
}
