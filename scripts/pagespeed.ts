/**
 * PageSpeed audit runner for JapaConnect frontend surfaces.
 *
 * Engines (auto-selected):
 *   1. PageSpeed Insights API — used when JC_PAGESPEED_API_KEY (or
 *      PAGESPEED_API_KEY) is set. Returns lab data AND CrUX field data.
 *   2. Local Lighthouse (bunx lighthouse) — same lab engine PageSpeed
 *      uses; no key or quota required, needs Chrome/Chromium.
 *
 * Usage:
 *   bun run pagespeed                                    # all surfaces, mobile+desktop
 *   bun scripts/pagespeed.ts --strategy mobile           # one strategy
 *   bun scripts/pagespeed.ts --target main,docs          # subset
 *   bun scripts/pagespeed.ts --url main=http://localhost:3000
 *   bun scripts/pagespeed.ts --fail-under 90             # CI gate (perf score)
 *
 * Raw reports + summary.json land in reports/pagespeed/.
 */

import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';

interface Target {
	name: string;
	url: string;
}

const DEFAULT_TARGETS: Target[] = [
	{ name: 'main', url: 'https://japa.to' },
	{ name: 'docs', url: 'https://docs.japa.to/docs' },
	{ name: 'academy', url: 'https://academy.japa.to' }
];

const CATEGORIES = ['performance', 'accessibility', 'best-practices', 'seo'] as const;
const LIGHTHOUSE_VERSION = '13.0.1';

interface Cli {
	strategies: Array<'mobile' | 'desktop'>;
	targets: Target[];
	outDir: string;
	failUnder: number | null;
}

function parseArgs(argv: string[]): Cli {
	const cli: Cli = {
		strategies: ['mobile', 'desktop'],
		targets: DEFAULT_TARGETS,
		outDir: 'reports/pagespeed',
		failUnder: null
	};
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		const next = () => argv[++i];
		switch (arg) {
			case '--strategy':
			case '-s': {
				const v = next();
				cli.strategies = v === 'both' ? ['mobile', 'desktop'] : [v as 'mobile' | 'desktop'];
				break;
			}
			case '--target':
			case '-t': {
				const wanted = next().split(',');
				cli.targets = DEFAULT_TARGETS.filter((t) => wanted.includes(t.name));
				break;
			}
			case '--url': {
				const [name, url] = next().split(/=(.*)/s);
				const existing = cli.targets.find((t) => t.name === name);
				if (existing) existing.url = url;
				else cli.targets.push({ name, url });
				break;
			}
			case '--outdir':
			case '-o':
				cli.outDir = next();
				break;
			case '--fail-under':
				cli.failUnder = Number(next());
				break;
			case '--help':
			case '-h':
				console.log('bun scripts/pagespeed.ts [--strategy mobile|desktop|both] [--target main,docs,academy] [--url name=url]... [--outdir dir] [--fail-under N]');
				process.exit(0);
		}
	}
	if (cli.targets.length === 0) {
		console.error('no targets matched');
		process.exit(2);
	}
	return cli;
}

interface RunResult {
	target: string;
	strategy: string;
	url: string;
	engine: 'psi' | 'lighthouse';
	scores: Record<string, number | null>;
	metrics: Record<string, string>;
	fieldData: Record<string, string> | null;
	opportunities: Array<{ id: string; title: string; savingsMs: number }>;
	errored: boolean;
}

async function runPsi(target: Target, strategy: string, key: string, outDir: string): Promise<RunResult> {
	const qs = new URLSearchParams({ url: target.url, strategy, key });
	for (const c of CATEGORIES) qs.append('category', c);
	const res = await fetch(`https://www.googleapis.com/pagespeedonline/v5/runPagespeed?${qs}`);
	const body = (await res.json()) as any;
	if (!res.ok) {
		throw new Error(`PSI ${res.status}: ${body?.error?.message ?? 'unknown'}`);
	}
	writeFileSync(join(outDir, `${target.name}-${strategy}.json`), JSON.stringify(body, null, 2));

	const lhr = body.lighthouseResult;
	const loading = body.loadingExperience;
	return buildResult(target, strategy, 'psi', lhr, loading ?? null);
}

function runLighthouse(target: Target, strategy: string, outDir: string): Promise<RunResult> {
	const outPath = join(outDir, `${target.name}-${strategy}.json`);
	const args = [
		`lighthouse@${LIGHTHOUSE_VERSION}`,
		target.url,
		'--output=json',
		`--output-path=${outPath}`,
		`--only-categories=${CATEGORIES.join(',')}`,
		'--chrome-flags=--headless=new --no-sandbox --disable-gpu',
		'--quiet'
	];
	if (strategy === 'desktop') args.push('--preset=desktop');
	const run = spawnSync('bunx', args, { stdio: ['ignore', 'ignore', 'inherit'], timeout: 300_000 });
	if (run.error) throw run.error;
	if (run.status !== 0) throw new Error(`lighthouse exited ${run.status} for ${target.url}`);
	const lhr = JSON.parse(readFileSync(outPath, 'utf8'));
	return Promise.resolve(buildResult(target, strategy, 'lighthouse', lhr, null));
}

function buildResult(target: Target, strategy: string, engine: 'psi' | 'lighthouse', lhr: any, loading: any): RunResult {
	const scores: Record<string, number | null> = {};
	for (const [id, cat] of Object.entries<any>(lhr.categories ?? {})) {
		scores[id] = cat.score == null ? null : Math.round(cat.score * 100);
	}
	const pick = (id: string) => lhr.audits?.[id]?.displayValue ?? '';
	const metrics = {
		FCP: pick('first-contentful-paint'),
		LCP: pick('largest-contentful-paint'),
		TBT: pick('total-blocking-time'),
		CLS: pick('cumulative-layout-shift'),
		SI: pick('speed-index')
	};

	const fieldData: Record<string, string> = {};
	if (loading?.metrics) {
		fieldData.overall = loading.overall_category ?? '';
		for (const [k, v] of Object.entries<any>(loading.metrics)) {
			fieldData[k] = `${v.percentile} (${v.category})`;
		}
	}

	const opportunities = Object.entries<any>(lhr.audits ?? {})
		.filter(([, a]) => a.details?.type === 'opportunity' && a.score != null && a.score < 1)
		.map(([id, a]) => ({ id, title: a.title, savingsMs: a.details?.overallSavingsMs ?? 0 }))
		.sort((x, y) => y.savingsMs - x.savingsMs)
		.slice(0, 5);

	return { target: target.name, strategy, url: lhr.finalDisplayedUrl ?? target.url, engine, scores, metrics, fieldData, opportunities, errored: false };
}

function printTable(results: RunResult[]) {
	const pad = (s: unknown, n: number) => String(s ?? '-').padEnd(n);
	for (const r of results) {
		console.log(`\n${r.target} [${r.strategy}] ${r.url} (${r.engine})`);
		if (r.errored) continue;
		console.log(
			'  ' +
				['performance', 'accessibility', 'best-practices', 'seo']
					.map((c) => `${c}=${r.scores[c] ?? '-'}`)
					.join('  ')
		);
		console.log('  ' + Object.entries(r.metrics).map(([k, v]) => `${k}=${v || '-'}`).join('  '));
		if (r.fieldData && Object.keys(r.fieldData).length > 1) {
			console.log('  field(CrUX): ' + Object.entries(r.fieldData).map(([k, v]) => `${k}=${v}`).join('  '));
		}
		for (const o of r.opportunities) {
			console.log(`  opp ${(o.savingsMs / 1000).toFixed(2)}s  ${o.id}`);
		}
	}
	const rows = results.map((r) => [
		pad(r.target, 10),
		pad(r.strategy, 10),
		pad(r.scores.performance, 6),
		pad(r.scores.accessibility, 6),
		pad(r.scores['best-practices'], 6),
		pad(r.scores.seo, 6)
	]);
	console.log('\n' + pad('target', 10) + pad('strategy', 10) + pad('perf', 6) + pad('a11y', 6) + pad('bp', 6) + pad('seo', 6));
	for (const row of rows) console.log(row.join(''));
}

async function main() {
	const cli = parseArgs(process.argv.slice(2));
	const apiKey = process.env.JC_PAGESPEED_API_KEY || process.env.PAGESPEED_API_KEY || '';
	const engine: 'psi' | 'lighthouse' = apiKey ? 'psi' : 'lighthouse';
	mkdirSync(cli.outDir, { recursive: true });
	console.log(`engine: ${engine === 'psi' ? 'PageSpeed Insights API' : `local lighthouse@${LIGHTHOUSE_VERSION}`} (${cli.targets.length} targets x ${cli.strategies.length} strategies)`);

	const results: RunResult[] = [];
	for (const target of cli.targets) {
		for (const strategy of cli.strategies) {
			process.stderr.write(`auditing ${target.name} ${strategy}...\n`);
			try {
				results.push(engine === 'psi' ? await runPsi(target, strategy, apiKey, cli.outDir) : await runLighthouse(target, strategy, cli.outDir));
			} catch (err) {
				console.error(`  FAILED ${target.name}/${strategy}: ${(err as Error).message}`);
				results.push({
					target: target.name,
					strategy,
					url: target.url,
					engine,
					scores: {},
					metrics: {},
					fieldData: null,
					opportunities: [],
					errored: true
				});
			}
		}
	}

	printTable(results);
	writeFileSync(
		join(cli.outDir, 'summary.json'),
		JSON.stringify({ generatedAt: new Date().toISOString(), engine, results }, null, 2)
	);
	console.log(`\nreports written to ${cli.outDir}/`);

	if (cli.failUnder != null) {
		const failures = results.filter((r) => (r.scores.performance ?? 0) < cli.failUnder!);
		if (failures.length > 0) {
			console.error(`perf score below ${cli.failUnder}: ${failures.map((f) => `${f.target}/${f.strategy}`).join(', ')}`);
			process.exit(1);
		}
	}
	if (results.every((r) => r.errored)) process.exit(1);
}

await main();
