// Shared transient-notification (toast) store for all JapaConnect apps.
// Mount <ToastHost/> once in the root layout, then call toast.*() anywhere —
// API clients, stores, components. Module-level runes state is app-scoped
// because every app bundles its own copy of this module.

export type ToastVariant = 'info' | 'success' | 'warning' | 'error';

export interface ToastItem {
	id: string;
	variant: ToastVariant;
	message: string;
}

interface ToastOptions {
	/** Auto-dismiss after this many ms. 0 = sticky. Default 5000 (8000 for errors). */
	duration?: number;
}

let toasts = $state<ToastItem[]>([]);
const timers = new Map<string, ReturnType<typeof setTimeout>>();

export function getToasts(): ToastItem[] {
	return toasts;
}

export function dismissToast(id: string): void {
	toasts = toasts.filter((t) => t.id !== id);
	const timer = timers.get(id);
	if (timer) {
		clearTimeout(timer);
		timers.delete(id);
	}
}

function push(variant: ToastVariant, message: string, opts: ToastOptions = {}): string {
	if (!message) return '';
	// Collapse identical live toasts — a polling call that keeps failing
	// (e.g. notifications every 30s) must not stack 30 copies of the same
	// message; the existing toast's timer keeps running.
	const existing = toasts.find((t) => t.variant === variant && t.message === message);
	if (existing) return existing.id;
	const id = crypto.randomUUID();
	toasts = [...toasts.slice(-4), { id, variant, message }];
	const duration = opts.duration ?? (variant === 'error' ? 8000 : 5000);
	if (duration > 0 && typeof window !== 'undefined') {
		timers.set(
			id,
			setTimeout(() => dismissToast(id), duration)
		);
	}
	return id;
}

export const toast = {
	info: (message: string, opts?: ToastOptions) => push('info', message, opts),
	success: (message: string, opts?: ToastOptions) => push('success', message, opts),
	warning: (message: string, opts?: ToastOptions) => push('warning', message, opts),
	error: (message: string, opts?: ToastOptions) => push('error', message, opts),
	dismiss: dismissToast
};
