// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
//
// Shared loader for the /messages/bookings + /messages/mentorship inboxes
// (task 11 — services/mentorship unification). Parses the URL params
// (role=sent|received → API booker|provider, status, intent, kind, page,
// b=<booking id> deep-link) and fetches GET /me/service-bookings plus the
// ?b= detail via GET /me/service-bookings/:id.
import type { RequestEvent } from '@sveltejs/kit';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ListMeta, ServiceBooking } from '$lib/types';

const BOOKING_STATUSES = new Set([
	'pending',
	'confirmed',
	'fulfilled',
	'cancelled',
	'declined',
	'disputed',
	'refunded'
]);
const BOOKING_INTENTS = new Set(['inquiry', 'booking']);
const BOOKING_KINDS = new Set(['service', 'package', 'mentorship']);
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const PAGE_LIMIT = 20;

export interface BookingsSurfaceOptions {
	/** Fixed listing kind for the surface (mentorship page passes 'mentorship'). */
	fixedKind?: string;
	/** Allow ?kind= pass-through filtering (bookings page). */
	allowKindFilter?: boolean;
	/** mentorship.enabled — role=provider serves user-provider rows only and
	 *  is mentorship-gated server-side; skip the call when the flag is off
	 *  instead of surfacing the endpoint's 404 as a load failure. */
	mentorshipEnabled: boolean;
}

export interface BookingsSurfaceData {
	bookings: ServiceBooking[];
	meta?: ListMeta;
	/** Booking resolved from ?b= — drives the detail pane. */
	selected: ServiceBooking | null;
	/** ?b= was present but the detail fetch failed (not found / not owner). */
	selectedMissing: boolean;
	loadError: boolean;
	role: 'sent' | 'received';
	status: string;
	intent: string;
	kind: string;
	currentPage: number;
}

export async function loadBookingsSurface(
	event: RequestEvent,
	opts: BookingsSurfaceOptions
): Promise<BookingsSurfaceData> {
	const ctx = apiContext(event);
	const params = event.url.searchParams;

	const role: 'sent' | 'received' = params.get('role') === 'received' ? 'received' : 'sent';
	const apiRole = role === 'received' ? 'provider' : 'booker';

	const statusRaw = params.get('status') ?? '';
	const status = BOOKING_STATUSES.has(statusRaw) ? statusRaw : '';
	const intentRaw = params.get('intent') ?? '';
	const intent = BOOKING_INTENTS.has(intentRaw) ? intentRaw : '';
	const kindRaw = params.get('kind') ?? '';
	const kind =
		opts.fixedKind ?? (opts.allowKindFilter && BOOKING_KINDS.has(kindRaw) ? kindRaw : '');
	const pageNum = Math.max(1, Math.floor(Number(params.get('page'))) || 1);
	const selectedId = params.get('b') ?? '';

	const q = new URLSearchParams();
	q.set('role', apiRole);
	if (status) q.set('status', status);
	if (intent) q.set('intent', intent);
	if (kind) q.set('kind', kind);
	q.set('page', String(pageNum));
	q.set('limit', String(PAGE_LIMIT));

	let bookings: ServiceBooking[] = [];
	let meta: ListMeta | undefined;
	let loadError = false;

	const providerAvailable = apiRole !== 'provider' || opts.mentorshipEnabled;
	if (providerAvailable) {
		try {
			const res = await apiList<ServiceBooking>(ctx, `/me/service-bookings?${q.toString()}`);
			bookings = res.items;
			meta = res.meta;
		} catch (err) {
			// A feature flag flipped off mid-flight answers 404 — show the
			// empty inbox rather than an error for an empty capability.
			if (err instanceof ApiError && err.status === 404) {
				bookings = [];
			} else {
				loadError = true;
			}
		}
	}

	let selected: ServiceBooking | null = null;
	let selectedMissing = false;
	if (selectedId && UUID_RE.test(selectedId)) {
		try {
			selected = await apiGet<ServiceBooking>(
				ctx,
				`/me/service-bookings/${encodeURIComponent(selectedId)}`
			);
		} catch {
			selectedMissing = true;
		}
	} else if (selectedId) {
		selectedMissing = true;
	}

	return {
		bookings,
		meta,
		selected,
		selectedMissing,
		loadError,
		role,
		status,
		intent,
		kind,
		currentPage: pageNum
	};
}
