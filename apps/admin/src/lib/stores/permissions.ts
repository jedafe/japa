import { writable } from 'svelte/store';

/** Resolved permission keys for the current admin user. */
export const permissions = writable<string[]>([]);

/** Whether permissions have been loaded from the backend. */
export const permissionsLoaded = writable(false);

/**
 * Check if the current user has a specific permission.
 * Returns false until permissions are loaded.
 */
export function hasPermission(permKey: string): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = perms.includes(permKey);
	});
	unsub();
	return result;
}

/**
 * Check if the current user has ALL of the given permissions.
 */
export function hasAllPermissions(permKeys: string[]): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = permKeys.every((k) => perms.includes(k));
	});
	unsub();
	return result;
}

/**
 * Check if the current user has ANY of the given permissions.
 */
export function hasAnyPermission(permKeys: string[]): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = permKeys.some((k) => perms.includes(k));
	});
	unsub();
	return result;
}

/**
 * Fetch the current user's permissions from the backend and update the store.
 * Should be called once after authentication.
 */
export async function fetchPermissions(token: string | null): Promise<void> {
	if (!token) {
		permissions.set([]);
		permissionsLoaded.set(true);
		return;
	}

	try {
		const apiBase = import.meta.env.VITE_API_URL ?? 'http://localhost:8080/api/v1';
		const res = await fetch(`${apiBase}/admin/rbac/user-permissions`, {
			headers: { Authorization: `Bearer ${token}` }
		});

		if (res.ok) {
			const body = await res.json();
			const perms = body.permissions ?? body.data?.permissions ?? [];
			permissions.set(perms);
		} else {
			permissions.set([]);
		}
	} catch {
		permissions.set([]);
	} finally {
		permissionsLoaded.set(true);
	}
}
