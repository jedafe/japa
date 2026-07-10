import { writable, derived } from 'svelte/store';
import type { User } from '$lib/api/types';

export const user = writable<User | null>(null);
export const isAuth = derived(user, ($user) => !!$user);

export function setUser(u: User | null) {
	user.set(u);
}

export function clearUser() {
	user.set(null);
}
