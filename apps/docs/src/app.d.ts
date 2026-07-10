import type { User } from '$lib/api/types';

declare global {
	namespace App {
		interface Error {
			code: string;
			message: string;
		}
		interface Locals {
			user: User | null;
			token: string | null;
			isAuth: boolean;
			featureFlags: Record<string, boolean>;
		}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
