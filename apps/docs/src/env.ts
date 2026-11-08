import { defineEnvVars } from '@sveltejs/kit/env';

// @migration-task Review usage of dynamic environment variables. They fall back to the empty string if not present, which may not be what you want.
export const variables = defineEnvVars({
	JC_ENV: { schema: (input) => input ?? '' },
	PUBLIC_JC_ENV: { schema: (input) => input ?? '' },
	PUBLIC_JC_API_URL: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_UMAMI_URL: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_UMAMI_SITE_ID_DOCS: { public: true, schema: (input) => input ?? '' },
	PUBLIC_MAIN_ORIGIN: { public: true, schema: (input) => input ?? '' },
	PRIVATE_API_BASE_URL: { schema: (input) => input ?? '' },
	PUBLIC_DOCS_ORIGIN: { schema: (input) => input ?? '' }
});
