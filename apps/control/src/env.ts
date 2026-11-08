import { defineEnvVars } from '@sveltejs/kit/env';

// @migration-task Review usage of dynamic environment variables. They fall back to the empty string if not present, which may not be what you want.
export const variables = defineEnvVars({
	PUBLIC_JC_GLITCHTIP_DSN: { public: true, static: true },
	PUBLIC_JC_UMAMI_URL: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_UMAMI_SITE_ID_CONTROL: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_API_URL: { public: true, static: true },
	PUBLIC_JC_MAIN_URL: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_ENV: { public: true, static: true }
});
