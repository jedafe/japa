// Shared UI state for the guest journey onboarding modal.
// Ref: 2026-08-21-journey-aware-recommendations-design.md §6.1

export const journeyModal = $state({ open: false });

/** One minute in milliseconds. */
export const JOURNEY_MODAL_DELAY_MS = 1 * 60 * 1000;
