import { get, writable } from 'svelte/store';
import { apiFetch } from '$lib/api/client';

export const permissions = writable<string[]>([]);
export const permissionsLoaded = writable(false);

// `'*'` is the super-admin wildcard granted by the backend RBAC resolver;
// every check must honor it or wildcard admins see a bricked UI.
export function hasPermission(permKey: string): boolean {
	const perms = get(permissions);
	return perms.includes(permKey) || perms.includes('*');
}

export function hasAllPermissions(permKeys: string[]): boolean {
	return permKeys.every((k) => hasPermission(k));
}

export function hasAnyPermission(permKeys: string[]): boolean {
	return permKeys.some((k) => hasPermission(k));
}

export async function fetchPermissions(): Promise<string[]> {
	const result = await apiFetch<{ permissions: string[] }>('/me/permissions');

	const perms = result.data?.permissions ?? [];
	permissions.set(perms);
	permissionsLoaded.set(true);
	return perms;
}
