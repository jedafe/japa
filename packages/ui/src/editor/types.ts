/**
 * Shared types for the reusable TipTap rich-text editor (extracted from the
 * main app's PostEditor — sections/15-tiptap-rich-text.md).
 */

/** One autocomplete result for @ / [[ / # / {{ triggers. */
export interface EditorSearchHit {
	id: string;
	label: string;
	slug?: string;
	handle?: string;
	avatar_url?: string | null;
	/** Public path the reference links to at render time. */
	url?: string;
}

export type EditorMode = 'block' | 'wysiwyg' | 'markdown';

/** Per-app i18n hook — matches the apps' `t(key, fallback?)` signature. */
export type EditorTranslator = (key: string, fallback?: string) => string;

/** Link-preview metadata as returned by a host-provided fetcher. */
export interface LinkPreviewData {
	url: string;
	title: string;
	description: string;
	image: string;
	site_name: string;
	favicon: string;
}

/** Optional capabilities the host app can turn on. */
export interface EditorFeatures {
	/** @ mention / # hashtag / [[ interlink / {{ literal pickers — requires onSearch. */
	suggestions?: boolean;
	/** Coupon embed block (Section 55). Requires onSearchCoupons for the modal. */
	coupon?: boolean;
	/** Link-preview block node + card hydration. Requires fetchLinkPreview. */
	linkPreview?: boolean;
	/** §106 video_sources embed node support (appendVideoNode). */
	video?: boolean;
	/** Image insertion — via onPickImage callback, or a URL prompt modal. */
	image?: boolean;
	/** YouTube embeds. */
	youtube?: boolean;
	/** Markdown mode tab + getMarkdown/setContent markdown round-trip. */
	markdown?: boolean;
	/** §45 FAQ accordion block (details/summary markup). On unless false. */
	faq?: boolean;
}
