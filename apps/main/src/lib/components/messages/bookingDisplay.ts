// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
// Shared display helpers for the /messages/bookings + /messages/mentorship
// panes — status vocabulary, counterpart resolution, date formatting.
import { t } from '$lib/i18n';
import type { ServiceBooking } from '$lib/types';

/** Display status — mentorship-kind rows carry the legacy
 *  mentorship_connections vocabulary (confirmed→active, fulfilled→completed)
 *  in connection_status; prefer it when present. */
export function bookingStatus(b: ServiceBooking): string {
	return b.connection_status || b.status;
}

/** Chip label — active/completed live under mentors.status_*; the booking
 *  vocabulary lives under bookings.status_*. */
export function bookingStatusLabel(b: ServiceBooking): string {
	const s = bookingStatus(b);
	if (s === 'active' || s === 'completed') return t(`mentors.status_${s}`, s);
	return t(`bookings.status_${s}`, s);
}

export function bookingStatusColor(status: string): string {
	switch (status) {
		case 'confirmed':
		case 'fulfilled':
		case 'active':
		case 'completed':
			return 'text-[var(--color-success)]';
		case 'pending':
			return 'text-[var(--color-info)]';
		case 'disputed':
			return 'text-[var(--color-warning,var(--color-info))]';
		case 'cancelled':
		case 'declined':
			return 'text-[var(--color-danger)]';
		default:
			return 'text-[var(--color-muted)]';
	}
}

export interface BookingCounterpart {
	name: string;
	username: string;
	avatar: string;
}

/**
 * The "other party" on a booking row. `sent` (booker view) shows the
 * provider — org_name for package bookings, the mentor's display name for
 * user-provider rows; `received` (provider view) shows the booker.
 */
export function bookingCounterpart(b: ServiceBooking, role: 'sent' | 'received'): BookingCounterpart {
	if (role === 'received') {
		return {
			name: b.booker_display_name || b.booker_username || '',
			username: b.booker_username ?? '',
			avatar: b.booker_avatar ?? ''
		};
	}
	return {
		name:
			b.org_name ||
			b.provider_display_name ||
			b.provider_username ||
			b.booker_display_name ||
			'',
		username: b.provider_username ?? '',
		avatar: b.provider_avatar ?? ''
	};
}

export function formatBookingDate(iso?: string): string {
	if (!iso) return '';
	return new Date(iso).toLocaleDateString(undefined, {
		year: 'numeric',
		month: 'short',
		day: 'numeric'
	});
}
