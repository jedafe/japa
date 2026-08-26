import { readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

function bumpPatch(version: string): string {
	const parts = version.split('.').map(Number);
	parts[parts.length - 1]++;
	return parts.join('.');
}

const root = join(dirname(fileURLToPath(import.meta.url)), '..');

// tauri.conf.json
const tauriPath = join(root, 'src-tauri', 'tauri.conf.json');
const tauriConf = JSON.parse(await readFile(tauriPath, 'utf8'));
tauriConf.version = bumpPatch(tauriConf.version);
await writeFile(tauriPath, JSON.stringify(tauriConf, null, 2) + '\n');

// Cargo.toml
const cargoPath = join(root, 'src-tauri', 'Cargo.toml');
let cargoToml = await readFile(cargoPath, 'utf8');
cargoToml = cargoToml.replace(
	/^(version\s*=\s*")([^"]+)(")/m,
	(_: string, pre: string, v: string, post: string) => `${pre}${bumpPatch(v)}${post}`
);
await writeFile(cargoPath, cargoToml);

// package.json
const packagePath = join(root, 'package.json');
const pkg = JSON.parse(await readFile(packagePath, 'utf8'));
pkg.version = bumpPatch(pkg.version);
await writeFile(packagePath, JSON.stringify(pkg, null, '\t') + '\n');

console.log(`Bumped version to ${tauriConf.version}`);
