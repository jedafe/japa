import { defineEnvVars } from '@sveltejs/kit/env';

// @migration-task Review usage of dynamic environment variables. They fall back to the empty string if not present, which may not be what you want.
export const variables = defineEnvVars({
	JC_ENV: { schema: (input) => input ?? '' },
	PUBLIC_JC_ENV: { schema: (input) => input ?? '' },
	PUBLIC_JC_API_URL: { public: true, static: true },
	PUBLIC_JC_UMAMI_URL: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_UMAMI_SITE_ID_ACADEMY: { public: true, schema: (input) => input ?? '' },
	PUBLIC_MAIN_ORIGIN: { public: true, static: true },
	PUBLIC_JC_ACADEMY_ORIGIN: { public: true, schema: (input) => input ?? '' },
	PRIVATE_API_BASE_URL: { schema: (input) => input ?? '' }
});
