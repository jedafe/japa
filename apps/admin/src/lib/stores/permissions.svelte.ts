import { apiFetch } from '$lib/api/client';

let _permissions = $state<string[]>([]);
let _permissionsLoaded = $state(false);

// `'*'` is the super-admin wildcard granted by the backend RBAC resolver;
// every check must honor it or wildcard admins see a bricked UI.
export function hasPermission(permKey: string): boolean {
	return _permissions.includes(permKey) || _permissions.includes('*');
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
	_permissions = perms;
	_permissionsLoaded = true;
	return perms;
}

export function setPermissions(perms: string[]) {
	_permissions = perms;
}

export function getPermissions(): string[] {
	return _permissions;
}

export function isPermissionsLoaded(): boolean {
	return _permissionsLoaded;
}
