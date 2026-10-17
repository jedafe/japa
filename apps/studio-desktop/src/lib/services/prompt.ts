/**
 * Build a Claude Code prompt from prompt builder form values.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import {
	FRAMEWORK_BY_CONTENT_TYPE,
	DEFAULT_PROMPT_VALUES,
	type PromptFormValues,
	type SelectOption
} from '$lib/types/prompt';
import { isHighStakes } from '$lib/services/validation';
import type { StudioMode } from '$lib/services/settings';
import * as yaml from 'js-yaml';
import { siteName } from '$lib/stores/site.svelte';

export interface PromptLookups {
	contentTypeName: string;
	guideCategoryName: string;
	personaName: string;
	voiceName: string;
	voicePrompt?: string;
	spaceName: string;
}

export function slugify(text: string): string {
	return text
		.toLowerCase()
		.trim()
		.replace(/[^\w\s-]/g, '')
		.replace(/\s+/g, '-')
		.replace(/-+/g, '-')
		.replace(/^-+|-+$/g, '');
}

/**
 * Auto-select a framework when a content type is chosen and the user
 * has not explicitly set one.
 * Ref: internal/ai/AiDraftPanel.svelte
 */
export function defaultFrameworkFor(contentType: string): string {
	return FRAMEWORK_BY_CONTENT_TYPE[contentType] ?? '';
}

function isHighStakesPrompt(values: PromptFormValues): boolean {
	return isHighStakes({ guide_category: values.guide_category });
}

function yamlMultiline(value: string): string {
	if (!value) return '';
	if (value.includes('\n')) {
		return `|\n    ${value.replace(/\n/g, '\n    ')}`;
	}
	return `"${value.replace(/"/g, '\\"')}"`;
}

function frontmatterLine(key: string, value: unknown): string {
	if (value === '' || value === null || value === undefined) return '';
	if (typeof value === 'boolean') return `${key}: ${value}`;
	if (typeof value === 'number') return `${key}: ${value}`;
	if (typeof value === 'string') return `${key}: ${yamlMultiline(value)}`;
	return '';
}

function effectiveStatus(values: PromptFormValues): string {
	if (isHighStakesPrompt(values) && (values.status === 'published' || values.status === 'scheduled')) {
		return 'in_review';
	}
	return values.status;
}

function fieldValuesYaml(values: Record<string, string>): string {
	const entries = Object.entries(values).filter(([, v]) => v !== '');
	if (entries.length === 0) return '';
	const dumped = yaml.dump(Object.fromEntries(entries), { lineWidth: -1, indent: 2 }).trim();
	return `field_values:\n${dumped
		.split('\n')
		.map((line) => `    ${line}`)
		.join('\n')}\n`;
}

export function buildPrompt(values: PromptFormValues, lookups: PromptLookups, mode?: StudioMode): string {
	const slug = slugify(values.title || values.topic || 'untitled');
	const filePath = `jc-content/${values.content_type}/${slug}.md`;
	const status = effectiveStatus(values);

	const frameworkInstruction = values.framework
		? `Use the ${values.framework.toUpperCase()} framework to structure the article.`
		: '';

	const structuralParts: string[] = [];
	if (values.include_intro) structuralParts.push('Include a compelling introduction.');
	if (values.include_conclusion) structuralParts.push('End with a strong conclusion.');
	if (values.include_faqs) structuralParts.push('Add a short FAQ section at the end.');
	if (values.include_steps) structuralParts.push('Use numbered steps where applicable.');
	if (values.include_key_takeaways) structuralParts.push('End each major section with key takeaways.');
	if (values.include_inline_image) structuralParts.push('Suggest relevant inline image placeholders with descriptive alt text.');

	const linkInstructions: string[] = [];
	if (values.internal_links > 0) {
		linkInstructions.push(
			`Include ${values.internal_links} internal links in the form [text](/content/[slug]). Do not invent slugs; use real ${siteName()} content slugs only if you know them, otherwise omit the link.`
		);
	}
	if (values.external_links > 0) {
		linkInstructions.push(
			`Include ${values.external_links} high-quality external links with real, working URLs. Only use authoritative sources.`
		);
	}

	const constraints: string[] = [];
	if (values.min_words > 0) {
		constraints.push(`Minimum word count: ${values.min_words}`);
	}
	if (values.heading_count > 0) {
		constraints.push(
			`Minimum number of H2 headings: ${values.heading_count}. Headings count excludes introduction, conclusion, FAQs, and key takeaways.`
		);
	}
	if (values.min_paragraphs_per_heading > 0) {
		constraints.push(`At least ${values.min_paragraphs_per_heading} paragraphs per heading.`);
	}
	if (values.paragraph_count > 0) {
		constraints.push(`Minimum total paragraphs (approximate): ${values.paragraph_count}.`);
	}

	const humanizeInstruction =
		values.humanize_level === 'off'
			? ''
			: `Write in a way that sounds ${values.humanize_level === 'heavy' ? 'very ' : ''}natural and human. Avoid obvious AI phrasing and generic filler.`;

	const modeInstruction: Record<string, string> = {
		generate: `Write a new article on the topic. Save it to ${filePath}.`,
		improve: `Improve the existing article at ${filePath}. Keep the same frontmatter unless instructed otherwise, and rewrite the body for clarity, flow, and accuracy.`,
		extend: `Expand the existing article at ${filePath}. Add depth, new sections, and updated information while keeping the existing frontmatter unless it needs correction.`,
		shorten: `Shorten the existing article at ${filePath}. Preserve the key points and frontmatter, but make it more concise.`
	};

	const tagsYaml =
		values.tags.length > 0
			? `tags:\n${values.tags.map((t) => `  - ${yamlMultiline(t)}`).join('\n')}`
			: 'tags: []';

	const isMember = mode === 'member';

	const voiceBlock = isMember || !lookups.voicePrompt
		? ''
		: `Voice guidance: ${lookups.voicePrompt}`;
	const authorBlock = isMember
		? `You are a ${siteName()} member writing under your own profile.`
		: `You are writing as the ${siteName()} persona "${lookups.personaName || `a ${siteName()} author`}". ${voiceBlock}`;

	const publishAt = values.publish_at ? `publish_at: "${values.publish_at}"\n` : '';
	const topicLine = values.topic || values.title;

	return `${authorBlock}

Topic: ${topicLine || '(not provided)'}
Audience: ${values.audience || `general ${siteName()} readers`}
Content type: ${lookups.contentTypeName || values.content_type}${lookups.guideCategoryName ? ` / ${lookups.guideCategoryName}` : ''}
Tone variant: ${values.tone}
Voice: ${lookups.voiceName || '(not selected)'}
Humanize: ${values.humanize_level}
${constraints.length > 0 ? `${constraints.join('\n')}\n` : ''}${frameworkInstruction ? `${frameworkInstruction}\n` : ''}
${modeInstruction[values.mode]}

Structure and content rules:
- Use Markdown. H1 for the title, H2 for each major section.
${structuralParts.map((p) => `- ${p}`).join('\n')}
${linkInstructions.map((p) => `- ${p}`).join('\n')}
${humanizeInstruction ? `- ${humanizeInstruction}` : ''}
- Do not sound generic or AI-like. Be specific, practical, and human.

Required frontmatter (fill in real values; do not leave placeholders empty):

\`\`\`yaml
---
${frontmatterLine('title', values.title || values.topic || '')}
${frontmatterLine('slug', slug)}
${frontmatterLine('content_type', values.content_type)}
${values.guide_category ? frontmatterLine('guide_category', values.guide_category) : ''}${frontmatterLine('source_country', values.source_country)}
${frontmatterLine('target_country', values.target_country)}
${frontmatterLine('journey_stage', values.journey_stage)}
${frontmatterLine('visa_route', values.visa_route)}
${frontmatterLine('space_id', values.space_id)}
${isMember ? '' : frontmatterLine('persona_id', values.persona_id)}${isMember ? '' : frontmatterLine('voice_id', values.voice_id)}${frontmatterLine('status', status)}
${frontmatterLine('visibility', values.visibility)}
${publishAt}${tagsYaml}
${fieldValuesYaml(values.field_values)}${frontmatterLine('header_image_id', values.header_image_id)}
${frontmatterLine('featured_image_id', values.featured_image_id)}
${frontmatterLine('meta_title', values.meta_title)}
${frontmatterLine('meta_description', values.meta_description)}
${frontmatterLine('canonical_url', values.canonical_url)}
${frontmatterLine('og_image_url', values.og_image_url)}
${frontmatterLine('seo_indexable', values.seo_indexable)}
---
\`\`\`
`;
}

export function buildProjectInstruction(promptText: string): string {
	return `# Japa Content Studio — Project Instructions

You are the Japa Content Studio writing assistant. Your job is to help the user create high-quality, human-sounding ${siteName()} content from the prompt they provide.

When given a generation prompt, write the full article as a Markdown file under \`jc-content/[content-type]/[slug].md\` with the exact frontmatter requested. Always use H1 for the title and H2 for section headings. Include internal links as \`[text](/content/[slug])\` and real external URLs where requested. Fill all frontmatter fields with real, accurate values; do not leave placeholder text.

## Current prompt

${promptText}
`;
}

export function optionFromList(options: SelectOption[], value: string): SelectOption | undefined {
	return options.find((o) => o.value === value);
}
