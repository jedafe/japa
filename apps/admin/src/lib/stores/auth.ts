import { writable, derived, get } from 'svelte/store';
import type { User } from '$lib/api/types';
// Single source of truth for the permission list. Both this module and
// $lib/stores/permissions expose the SAME store — a second copy here once
// left the layout gate reading an always-empty list (AccessDenied for all
// users; see production-audit-2026-07-11.md B3).
import { permissions } from '$lib/stores/permissions';

export { permissions };

export const user = writable<User | null>(null);
export const isAuth = derived(user, ($user) => !!$user);

export function setUser(u: User | null) {
	user.set(u);
}

export function setPermissions(perms: string[]) {
	permissions.set(perms);
}

export function clearUser() {
	user.set(null);
	permissions.set([]);
}

export function hasPermission(node: string): boolean {
	const perms = get(permissions);
	return perms.includes(node) || perms.includes('*');
}

export function hasAnyPermission(nodes: string[]): boolean {
	return nodes.some((node) => hasPermission(node));
}

export function hasAllPermissions(nodes: string[]): boolean {
	return nodes.every((node) => hasPermission(node));
}
