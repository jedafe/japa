import type { User } from '$lib/api/types';

let user = $state<User | null>(null);
let permissions = $state<string[]>([]);

export function getUser() {
	return user;
}

export function getPermissions() {
	return permissions;
}

export function setUser(u: User | null) {
	user = u;
}

export function setPermissions(perms: string[]) {
	permissions = perms;
}

export function clearUser() {
	user = null;
	permissions = [];
}

export function hasPermission(node: string): boolean {
	return permissions.includes(node) || permissions.includes('*');
}

export function hasAnyPermission(nodes: string[]): boolean {
	return nodes.some((node) => hasPermission(node));
}

export function hasAllPermissions(nodes: string[]): boolean {
	return nodes.every((node) => hasPermission(node));
}
