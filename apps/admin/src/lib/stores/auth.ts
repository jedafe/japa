import { writable, derived, get } from 'svelte/store';
import type { User } from '$lib/api/types';

export const user = writable<User | null>(null);
export const permissions = writable<string[]>([]);
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
