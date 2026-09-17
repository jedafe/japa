// Ref: .jeos/sections/106-video-content-and-import.md — the canonical studio
// route is /studio (singular). /studios is a common mistyped/bookmarked
// variant; redirect it so users land on the real page instead of a 404.
import { redirect } from '@sveltejs/kit';

export function load() {
	redirect(308, '/studio');
}
