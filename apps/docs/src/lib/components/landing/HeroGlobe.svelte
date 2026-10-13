<script lang="ts">
	// Decorative hero illustration — a stylized globe with orbiting docs
	// domains (API, Members, Analytics, Security). Pure SVG/CSS so it follows
	// the theme tokens and needs no image assets. aria-hidden: decoration only.
	import { Code2, Users, BarChart3, ShieldCheck, Send } from 'lucide-svelte';
</script>

<div class="globe-wrap" aria-hidden="true">
	<svg class="globe" viewBox="0 0 360 360" fill="none" role="img">
		<defs>
			<radialGradient id="globe-grad" cx="0.35" cy="0.3" r="0.9">
				<stop offset="0%" stop-color="var(--color-primary)" stop-opacity="0.22" />
				<stop offset="100%" stop-color="var(--color-primary)" stop-opacity="0.05" />
			</radialGradient>
		</defs>
		<!-- orbit ring -->
		<circle cx="180" cy="180" r="152" stroke="var(--color-border)" stroke-width="1" stroke-dasharray="4 6" />
		<!-- globe body -->
		<circle cx="180" cy="180" r="112" fill="url(#globe-grad)" stroke="var(--color-primary)" stroke-opacity="0.35" stroke-width="1" />
		<!-- meridians + parallels -->
		<ellipse cx="180" cy="180" rx="112" ry="44" stroke="var(--color-primary)" stroke-opacity="0.25" stroke-width="1" />
		<ellipse cx="180" cy="180" rx="112" ry="80" stroke="var(--color-primary)" stroke-opacity="0.18" stroke-width="1" />
		<ellipse cx="180" cy="180" rx="44" ry="112" stroke="var(--color-primary)" stroke-opacity="0.25" stroke-width="1" />
		<ellipse cx="180" cy="180" rx="80" ry="112" stroke="var(--color-primary)" stroke-opacity="0.18" stroke-width="1" />
		<line x1="68" y1="180" x2="292" y2="180" stroke="var(--color-primary)" stroke-opacity="0.25" stroke-width="1" />
		<!-- arc accents -->
		<path d="M 120 96 Q 180 60 240 96" stroke="var(--color-primary)" stroke-opacity="0.4" stroke-width="1.2" fill="none" stroke-linecap="round" />
		<path d="M 96 252 Q 180 300 264 252" stroke="var(--color-primary)" stroke-opacity="0.3" stroke-width="1.2" fill="none" stroke-linecap="round" />
	</svg>

	<span class="plane"><Send size={20} /></span>

	<div class="float-card fc-api">
		<span class="fc-icon"><Code2 size={16} /></span>
		<span class="fc-label">API</span>
	</div>
	<div class="float-card fc-members">
		<span class="fc-icon"><Users size={16} /></span>
		<span class="fc-label">Members</span>
	</div>
	<div class="float-card fc-analytics">
		<span class="fc-icon"><BarChart3 size={16} /></span>
		<span class="fc-label">Analytics</span>
	</div>
	<div class="float-card fc-security">
		<span class="fc-icon"><ShieldCheck size={16} /></span>
		<span class="fc-label">Security</span>
	</div>
</div>

<style>
	.globe-wrap {
		position: relative;
		width: 100%;
		max-width: 24rem;
		aspect-ratio: 1;
	}
	.globe {
		display: block;
		width: 100%;
		height: 100%;
	}
	.plane {
		position: absolute;
		top: 4%;
		right: 12%;
		display: grid;
		place-items: center;
		height: 2.5rem;
		width: 2.5rem;
		border-radius: 50%;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		color: var(--color-primary);
		box-shadow: 0 4px 14px rgb(0 0 0 / 0.08);
	}
	.float-card {
		position: absolute;
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius-card);
		border: 1px solid var(--color-border);
		background: color-mix(in srgb, var(--color-surface) 85%, transparent);
		backdrop-filter: blur(4px);
		box-shadow: 0 6px 20px rgb(0 0 0 / 0.08);
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-text);
		animation: float 6s ease-in-out infinite;
	}
	.fc-icon {
		display: grid;
		place-items: center;
		height: 1.5rem;
		width: 1.5rem;
		border-radius: var(--radius);
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-primary);
	}
	.fc-api { top: 8%; left: 0; animation-delay: 0s; }
	.fc-members { top: 14%; right: -4%; animation-delay: 1.5s; }
	.fc-analytics { bottom: 18%; left: -2%; animation-delay: 3s; }
	.fc-security { bottom: 8%; right: 4%; animation-delay: 4.5s; }

	@keyframes float {
		0%, 100% { transform: translateY(0); }
		50% { transform: translateY(-8px); }
	}
	@media (prefers-reduced-motion: reduce) {
		.float-card { animation: none; }
	}
	@media (max-width: 900px) {
		.globe-wrap { max-width: 18rem; margin-inline: auto; }
	}
	@media (max-width: 640px) {
		.globe-wrap { display: none; }
	}
</style>
