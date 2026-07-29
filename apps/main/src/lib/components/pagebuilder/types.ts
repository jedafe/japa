/**
 * Page Builder block types (Section 16 / ADR-011).
 * A landing page is an ordered array of block instances, each with a type
 * discriminator and a props object whose shape depends on the type.
 */

export interface LandingPage {
	id: string;
	title: string;
	slug: string;
	description?: string | null;
	blocks: LandingBlock[];
	meta?: Record<string, unknown>;
}

export interface LandingBlock {
	id?: string;
	type: string;
	props: Record<string, unknown>;
}

// ─── Block prop shapes (the most common landing blocks) ───────────────────

export interface HeroProps {
	title: string;
	subtitle?: string;
	cta_text?: string;
	cta_url?: string;
	background_image?: string;
	overlay_opacity?: number;
	text_align?: 'left' | 'center' | 'right';
}

export interface RichTextProps {
	content: string;
}

export interface CtaButtonProps {
	text: string;
	url: string;
	style?: 'primary' | 'secondary' | 'outline';
	align?: 'left' | 'center' | 'right';
	open_new_tab?: boolean;
}

export interface StatsProps {
	items: Array<{ label: string; value: string; prefix?: string; suffix?: string }>;
}

export interface SpacerDividerProps {
	height_px?: number;
	show_line?: boolean;
}

export interface DynamicListingProps {
	widget_id?: string;
	content_type?: string;
	limit?: number;
}

// ─── Phase 27.02-27.06 — additional block prop shapes ────────────────────

export interface ImageGalleryProps {
	images: Array<{ url: string; alt?: string; caption?: string }>;
	columns?: 2 | 3 | 4;
}

export interface SliderProps {
	slides: Array<{ image_url?: string; title?: string; subtitle?: string; cta_text?: string; cta_url?: string }>;
	autoplay?: boolean;
	interval_ms?: number;
}

export interface FaqProps {
	items: Array<{ question: string; answer: string }>;
}

export interface TestimonialProps {
	quote: string;
	author_name: string;
	author_title?: string;
	avatar_url?: string;
	rating?: number;
}

export interface TableProps {
	headers: string[];
	rows: string[][];
	striped?: boolean;
}

export interface TocProps {
	title?: string;
	items: Array<{ label: string; anchor: string }>;
}

export interface CountdownProps {
	target_date: string;
	label?: string;
}

export interface ComparisonEmbedProps {
	compare_slug: string;
}

export interface FormEmbedProps {
	form_url: string;
	title?: string;
	height_px?: number;
}
