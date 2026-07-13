import { browser } from '$app/environment';

type ToastType = 'info' | 'success' | 'warning' | 'error';

interface Toast {
	id: string;
	type: ToastType;
	message: string;
	duration: number;
}

// Client-only state - safe for SSR because browser check guards all mutations
let toasts = $state<Toast[]>([]);
let counter = 0;

function generateId(): string {
	counter++;
	return `toast-${counter}-${Date.now()}`;
}

export function showToast(message: string, type: ToastType = 'info', duration = 5000) {
	// Only show toasts on client side
	if (!browser) return;
	
	const id = generateId();
	const toast: Toast = { id, type, message, duration };
	toasts = [...toasts, toast];

	if (duration > 0) {
		setTimeout(() => dismissToast(id), duration);
	}
}

export function dismissToast(id: string) {
	toasts = toasts.filter((t) => t.id !== id);
}

export function getToasts() {
	return {
		get current() {
			// During SSR, return empty array to avoid hydration mismatch
			return browser ? toasts : [];
		}
	};
}
