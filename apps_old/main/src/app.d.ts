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
				displayName: string | null;
				avatarUrl: string | null;
				profileType: string;
				status: string;
			} | null;
			token?: string;
			featureFlags: Record<string, boolean>;
			notificationCount: number;
		}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
