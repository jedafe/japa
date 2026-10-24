/**
 * Mobile FAB slot registry (client-only).
 *
 * The global Japa AI FAB owns slot 0 (bottom-20, end-4) on every app page.
 * PageAction FABs must never render there — the systemic overlap bug this
 * fixes — so each mounted PageAction claims the lowest free slot >= 1
 * (slot N sits N*3.5rem above the AI FAB). Explicit fabIndex props pin a
 * slot; mixed auto/explicit pages are collision-free as long as explicit
 * indexes mount no later than autos.
 *
 * Ref: ADR-058 (unified app shell list-page bar).
 */
const occupied = new Set<number>([0]);

export function claimFabSlot(preferred?: number): number {
	if (preferred !== undefined) {
		occupied.add(preferred);
		return preferred;
	}
	let i = 1;
	while (occupied.has(i)) i++;
	occupied.add(i);
	return i;
}

export function releaseFabSlot(slot: number): void {
	if (slot !== 0) occupied.delete(slot);
}
