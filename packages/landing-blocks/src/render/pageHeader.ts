// packages/landing-blocks/src/render/pageHeader.ts
//
// Page-header convention: when a page's first block is a `heading`, that
// block is the page title — render it as the shared hero panel (same look
// as the /tools/{slug} hero) instead of a bare heading, so every published
// page gets the same eyebrow + bold title + subtitle + artwork header
// whether the author used a `hero` block or a plain `heading`.
//
// An immediately-following `media` block folds into the hero as its artwork
// (pages like /privacy pair the title heading with a standalone
// illustration — in the hero layout that image becomes the right-side art).
//
// Applied inside LandingRenderer so the public page and the admin live
// preview stay identical.

import type { Block, HeadingProps, HeroProps, MediaBlockProps } from '../types';

/**
 * Returns a new block list with a leading `heading` block rewritten as a
 * `hero` block (inheriting the heading's id/style/visibility), and the next
 * block consumed as hero artwork when it is a `media` block with an image.
 * Returns the input unchanged when the first block is not a heading.
 */
export function applyPageHeader(blocks: Block[]): Block[] {
	const first = blocks[0];
	if (!first || first.type !== 'heading' || first.visible === false) return blocks;

	const p = first.props as HeadingProps;
	const heroProps: HeroProps = {
		headline: p.text ?? '',
		eyebrow: p.eyebrow,
		subhead: p.subheading,
		layout: 'split',
	};
	if (p.cta?.label && p.cta?.href) {
		heroProps.ctas = [{ label: p.cta.label, href: p.cta.href, variant: 'primary' }];
	}

	const rest = blocks.slice(1);
	const second = rest[0];
	if (second?.type === 'media' && second.visible !== false) {
		const mp = second.props as MediaBlockProps;
		if (mp.media?.url) {
			heroProps.media = mp.media;
			rest.shift();
		}
	}

	const hero = {
		...first,
		type: 'hero',
		props: heroProps,
	} as unknown as Block;
	return [hero, ...rest];
}
