import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /deals → 308 /products/deals — the deals listing moved under the
 * products surface as a tab (same fold-in pattern as /mentors →
 * /services?kind=mentorship). The whole query string carries over —
 * ?q=, ?provider_id=, ?type=, ?sort= and ?category= all map 1:1.
 *
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
 */
export const load: PageServerLoad = (event) => {
	redirect(308, `/products/deals${event.url.search}`);
};
