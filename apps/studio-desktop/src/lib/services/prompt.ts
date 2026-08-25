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

function fieldValuesYaml(values: PromptFormValues): string {
	const fv: Record<string, unknown> = {};
	if (values.source_country) fv.source_country = values.source_country;
	if (values.target_country) fv.target_country = values.target_country;
	if (values.journey_stage) fv.journey_stage = values.journey_stage;
	if (values.visa_route) fv.visa_route = values.visa_route;
	if (values.audience) fv.audience = values.audience;
	if (Object.keys(fv).length === 0) return '';
	return Object.entries(fv)
		.map(([k, v]) => `  ${k}: ${typeof v === 'string' ? `"${v}"` : v}`)
		.join('\n');
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

export function buildPrompt(values: PromptFormValues, lookups: PromptLookups): string {
	const slug = slugify(values.title || values.topic || 'untitled');
	const filePath = `jc-content/${values.content_type}/${slug}.md`;

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
			`Include ${values.internal_links} internal links in the form [text](/content/[slug]). Do not invent slugs; use real JapaConnect content slugs only if you know them, otherwise omit the link.`
		);
	}
	if (values.external_links > 0) {
		linkInstructions.push(
			`Include ${values.external_links} high-quality external links with real, working URLs. Only use authoritative sources.`
		);
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

	const fieldValuesYamlText = fieldValuesYaml(values);

	const voiceBlock = lookups.voicePrompt
		? `Voice guidance: ${lookups.voicePrompt}`
		: lookups.voiceName
			? `Voice: ${lookups.voiceName}`
			: '';

	const publishAt = values.publish_at ? `publish_at: "${values.publish_at}"\n` : '';

	return `You are writing as the JapaConnect persona "${lookups.personaName || 'a JapaConnect author'}". ${voiceBlock}

Topic: ${values.topic || values.title || 'TOPIC HERE'}
Audience: ${values.audience || 'general JapaConnect readers'}
Content type: ${lookups.contentTypeName || values.content_type}${lookups.guideCategoryName ? ` / ${lookups.guideCategoryName}` : ''}
Tone: ${values.tone}
Humanize: ${values.humanize_level}
Minimum word count: ${values.min_words}
Number of H2 headings: ${values.heading_count}
Paragraphs per heading: at least ${values.min_paragraphs_per_heading}
${values.paragraph_count ? `Total paragraphs (approximate): ${values.paragraph_count}\n` : ''}${frameworkInstruction}

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
${frontmatterLine('title', values.title || values.topic || 'TITLE HERE')}
${frontmatterLine('slug', slug || 'SLUG-HERE')}
${frontmatterLine('content_type', values.content_type)}
${values.guide_category ? frontmatterLine('guide_category', values.guide_category) : ''}${frontmatterLine('space_id', values.space_id || 'SPACE-ID-HERE')}
${frontmatterLine('persona_id', values.persona_id || 'PERSONA-ID-HERE')}
${frontmatterLine('voice_id', values.voice_id || 'VOICE-ID-HERE')}
${frontmatterLine('status', values.status)}
${frontmatterLine('visibility', values.visibility)}
${publishAt}${tagsYaml}
field_values:
  estimated_read_time:
${fieldValuesYamlText ? fieldValuesYamlText + '\n' : ''}${frontmatterLine('meta_title', values.meta_title)}
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

You are the Japa Content Studio writing assistant. Your job is to help the user create high-quality, human-sounding JapaConnect content from the prompt they provide.

When given a generation prompt, write the full article as a Markdown file under \`jc-content/[content-type]/[slug].md\` with the exact frontmatter requested. Always use H1 for the title and H2 for section headings. Include internal links as \`[text](/content/[slug])\` and real external URLs where requested. Fill all frontmatter fields with real, accurate values; do not leave placeholder text.

## Current prompt

${promptText}
`;
}

export function optionFromList(options: SelectOption[], value: string): SelectOption | undefined {
	return options.find((o) => o.value === value);
}
