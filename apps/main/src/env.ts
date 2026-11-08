import { defineEnvVars } from '@sveltejs/kit/env';

// @migration-task Review usage of dynamic environment variables. They fall back to the empty string if not present, which may not be what you want.
export const variables = defineEnvVars({
	JC_BASE_DOMAIN: { schema: (input) => input ?? '' },
	JC_ACADEMY_HOSTS: { schema: (input) => input ?? '' },
	JC_ROADMAP_HOSTS: { schema: (input) => input ?? '' },
	JC_ENV: { schema: (input) => input ?? '' },
	PUBLIC_JC_API_URL: { public: true, static: true },
	PUBLIC_JC_ENV: { schema: (input) => input ?? '' },
	PUBLIC_JC_UMAMI_URL: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JC_UMAMI_SITE_ID: { public: true, schema: (input) => input ?? '' },
	PRIVATE_API_BASE_URL: { schema: (input) => input ?? '' },
	PUBLIC_JCE_COMPARISON_LIST_PAGE_SIZE: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JCE_COMPARISON_LIST_MAX_PAGE_SIZE: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JCE_DEFAULT_ITEM_REF_TYPES: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JCE_DEFAULT_PAGE_SIZE: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JCE_DEFAULT_PAGE_SIZE_OPTIONS: { public: true, schema: (input) => input ?? '' },
	PUBLIC_JCE_DEFAULT_SORT_OPTIONS: { public: true, schema: (input) => input ?? '' },
	PRIVATE_COOKIE_SECURE: { schema: (input) => input ?? '' },
	PRIVATE_API_PUBLIC_ORIGIN: { schema: (input) => input ?? '' }
});
