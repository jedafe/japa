// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	namespace App {
		interface Error {
			message: string;
		}
		interface Locals {
			token: string | null;
			user: {
				id: string;
				email: string;
				username: string;
				displayName: string;
			} | null;
		}
		interface PageData {
			token: string | null;
			user: App.Locals['user'];
		}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
