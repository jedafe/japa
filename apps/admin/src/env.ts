import { defineEnvVars } from '@sveltejs/kit/env';

export const variables = defineEnvVars({
	PUBLIC_JC_GLITCHTIP_DSN: { public: true, static: true },
	PUBLIC_JC_UMAMI_URL: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_UMAMI_SITE_ID_ADMIN: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_API_URL: { public: true, static: true },
	PUBLIC_JC_SITE_URL: { public: true, static: true },
	PUBLIC_JC_WS_URL: { public: true, static: true },
	PRIVATE_API_BASE_URL: { schema: (input) => input ?? '' },
	PUBLIC_JC_ENV: { public: true, static: true }
});
