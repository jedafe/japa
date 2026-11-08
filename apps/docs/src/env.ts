import { defineEnvVars } from '@sveltejs/kit/env';

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
