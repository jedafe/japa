import { PUBLIC_JC_SITE_URL } from '$env/static/public';

const SITE_URL = PUBLIC_JC_SITE_URL || 'https://japaconnect.com';

export interface PageMetadata {
	id?: string;
	title: string;
	description: string;
	slug?: string;
	content_type?: string;
	og_image?: string;
	canonical_url?: string;
	author?: string;
	published_at?: string;
	updated_at?: string;
}

export interface JsonLdSchema {
	'@context': string;
	'@type': string;
	[key: string]: unknown;
}

/**
 * Create Article schema for blog posts and content
 */
export function createArticleSchema(metadata: PageMetadata): JsonLdSchema {
	return {
		'@context': 'https://schema.org',
		'@type': 'Article',
		headline: metadata.title,
		description: metadata.description,
		image: metadata.og_image,
		author: {
			'@type': 'Person',
			name: metadata.author || 'JapaConnect User'
		},
		publisher: {
			'@type': 'Organization',
			name: 'JapaConnect',
			logo: {
				'@type': 'ImageObject',
				url: '/logo.png'
			}
		},
		datePublished: metadata.published_at,
		dateModified: metadata.updated_at,
		mainEntityOfPage: {
			'@type': 'WebPage',
			'@id': metadata.canonical_url || `${SITE_URL}/${metadata.slug}`
		}
	};
}

/**
 * Create Organization schema for the platform
 */
export function createOrganizationSchema(): JsonLdSchema {
	return {
		'@context': 'https://schema.org',
		'@type': 'Organization',
		name: 'JapaConnect',
		description: 'Community platform for the Nigerian diaspora',
		url: SITE_URL,
		logo: `${SITE_URL}/logo.png`,
		sameAs: [
			'https://twitter.com/japaconnect',
			'https://facebook.com/japaconnect',
			'https://linkedin.com/company/japaconnect'
		]
	};
}

/**
 * Create BreadcrumbList schema for navigation
 */
export function createBreadcrumbSchema(
	items: Array<{ name: string; url: string }>
): JsonLdSchema {
	return {
		'@context': 'https://schema.org',
		'@type': 'BreadcrumbList',
		itemListElement: items.map((item, index) => ({
			'@type': 'ListItem',
			position: index + 1,
			name: item.name,
			item: item.url
		}))
	};
}

/**
 * Create WebSite schema for the homepage
 */
export function createWebsiteSchema(): JsonLdSchema {
	return {
		'@context': 'https://schema.org',
		'@type': 'WebSite',
		name: 'JapaConnect',
		description: 'Community platform for the Nigerian diaspora',
		url: SITE_URL,
		potentialAction: {
			'@type': 'SearchAction',
			target: `${SITE_URL}/search?q={search_term_string}`,
			'query-input': 'required name=search_term_string'
		}
	};
}

/**
 * Create FAQ schema for FAQ pages
 */
export function createFaqSchema(
	faqs: Array<{ question: string; answer: string }>
): JsonLdSchema {
	return {
		'@context': 'https://schema.org',
		'@type': 'FAQPage',
		mainEntity: faqs.map((faq) => ({
			'@type': 'Question',
			name: faq.question,
			acceptedAnswer: {
				'@type': 'Answer',
				text: faq.answer
			}
		}))
	};
}