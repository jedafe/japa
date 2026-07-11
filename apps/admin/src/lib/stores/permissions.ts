import { writable } from 'svelte/store';
import { apiFetch } from '$lib/api/client';

export const permissions = writable<string[]>([]);
export const permissionsLoaded = writable(false);

export function hasPermission(permKey: string): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = perms.includes(permKey);
	});
	unsub();
	return result;
}

export function hasAllPermissions(permKeys: string[]): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = permKeys.every((k) => perms.includes(k));
	});
	unsub();
	return result;
}

export function hasAnyPermission(permKeys: string[]): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = permKeys.some((k) => perms.includes(k));
	});
	unsub();
	return result;
}

export async function fetchPermissions(_token?: string | null): Promise<void> {
	const result = await apiFetch<{ permissions: string[] }>('/me/permissions');

	if (result.data) {
		permissions.set(result.data.permissions ?? []);
	} else {
		permissions.set([]);
	}
	permissionsLoaded.set(true);
}
