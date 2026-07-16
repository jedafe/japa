// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	namespace App {
		interface Error {
			code: string;
			message: string;
		}
		interface Locals {
			user: {
				id: string;
				email: string;
				username: string;
				displayName: string;
				groups?: string[];
				avatar_url?: string | null;
			} | null;
			token: string | null;
		}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
