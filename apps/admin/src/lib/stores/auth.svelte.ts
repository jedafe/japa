import type { User } from '#lib/api/types.js';
import { setPermissions as _setPermissions } from '#lib/stores/permissions.svelte.js';

let _user = $state<User | null>(null);

export const user = {
	get current() { return _user; },
	set(val: User | null) { _user = val; }
};

export function getIsAuth(): boolean {
	return !!_user;
}

export function setUser(u: User | null) {
	_user = u;
}

export function setPermissions(perms: string[]) {
	_setPermissions(perms);
}

export function clearUser() {
	_user = null;
	_setPermissions([]);
}
