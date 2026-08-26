/**
 * Tauri binary end-to-end smoke test.
 * Builds the Tauri binary (debug, no bundle), starts tauri-driver and a native
 * WebDriver server, then drives the app with selenium-webdriver.
 *
 * On Linux this needs WebKitWebDriver in $PATH.
 * On Windows this needs msedgedriver in $PATH.
 * On macOS this needs safaridriver in $PATH.
 *
 * If the drivers are not available, the script exits 0 with a skip message so
 * CI can run it harmlessly.
 */

import { spawn, execFile } from 'node:child_process';
import { existsSync } from 'node:fs';
import path from 'node:path';
import { Builder, By, until } from 'selenium-webdriver';

const PORT = 4445;
const TIMEOUT = 120_000;

interface BinaryLocation {
	driver: string | null;
	tauriDriver: string | null;
}

function findInPath(name: string): Promise<string | null> {
	return new Promise((resolve) => {
		execFile('which', [name], (err, stdout) => {
			if (err) return resolve(null);
			resolve(stdout.trim() || null);
		});
	});
}

async function detectDrivers(): Promise<BinaryLocation> {
	const platform = process.platform;
	const tauriDriver =
		(await findInPath('tauri-driver')) ||
		(path.join(process.env.HOME ?? '', '.cargo', 'bin', 'tauri-driver') && existsSync(path.join(process.env.HOME ?? '', '.cargo', 'bin', 'tauri-driver'))
			? path.join(process.env.HOME ?? '', '.cargo', 'bin', 'tauri-driver')
			: null);

	let driver: string | null = null;
	if (platform === 'linux') {
		driver =
			(await findInPath('WebKitWebDriver')) ||
			'/usr/lib/x86_64-linux-gnu/webkit2gtk-4.1/WebKitWebDriver';
	} else if (platform === 'darwin') {
		driver = await findInPath('safaridriver');
	} else if (platform === 'win32') {
		driver = await findInPath('msedgedriver');
	}

	if (driver && !existsSync(driver)) driver = null;

	return { driver, tauriDriver };
}

function run(command: string, args: string[], cwd?: string): Promise<void> {
	return new Promise((resolve, reject) => {
		const child = spawn(command, args, { cwd, stdio: 'inherit' });
		child.on('close', (code) => {
			if (code === 0) resolve();
			else reject(new Error(`"${command} ${args.join(' ')}" exited with ${code}`));
		});
	});
}

function startProcess(command: string, args: string[], cwd?: string): ReturnType<typeof spawn> {
	return spawn(command, args, { cwd, stdio: 'pipe' });
}

async function waitForServer(url: string, attempts = 30): Promise<boolean> {
	for (let i = 0; i < attempts; i++) {
		try {
			const res = await fetch(url);
			if (res.ok || res.status === 405) return true;
		} catch {
			// not ready yet
		}
		await new Promise((r) => setTimeout(r, 500));
	}
	return false;
}

async function main(): Promise<void> {
	const { driver, tauriDriver } = await detectDrivers();

	if (!tauriDriver) {
		console.log('[e2e:tauri] tauri-driver not found; install it with `cargo install tauri-driver`. Skipping.');
		process.exit(0);
	}

	if (!driver) {
		console.log('[e2e:tauri] Native WebDriver server not found.');
		console.log('  Linux: install webkit2gtk-driver and ensure WebKitWebDriver is in $PATH.');
		console.log('  macOS: safaridriver must be in $PATH.');
		console.log('  Windows: msedgedriver must be in $PATH.');
		console.log('Skipping.');
		process.exit(0);
	}

	console.log(`[e2e:tauri] tauri-driver: ${tauriDriver}`);
	console.log(`[e2e:tauri] native driver: ${driver}`);

	const binaryPath = path.resolve(
		import.meta.dir,
		'..',
		'src-tauri',
		'target',
		'debug',
		'jc_studio'
	);

	if (!existsSync(binaryPath)) {
		console.log('[e2e:tauri] Building Tauri binary (debug, no bundle)...');
		await run('bunx', ['tauri', 'build', '--debug', '--no-bundle'], path.resolve(import.meta.dir, '..'));
	} else {
		console.log(`[e2e:tauri] Using existing binary: ${binaryPath}`);
	}

	const driverProcess = startProcess(tauriDriver, ['--port', String(PORT), '--native-driver', driver]);
	let webDriver: ReturnType<typeof Builder.prototype.build> | null = null;

	try {
		console.log('[e2e:tauri] Waiting for tauri-driver...');
		const ready = await waitForServer(`http://127.0.0.1:${PORT}/status`);
		if (!ready) throw new Error('tauri-driver did not become ready');

		console.log('[e2e:tauri] Starting WebDriver session...');
		webDriver = await new Builder()
			.usingServer(`http://127.0.0.1:${PORT}`)
			.withCapabilities({
				browserName: 'tauri',
				'tauri:options': {
					application: binaryPath
				}
			})
			.build();

		console.log('[e2e:tauri] Waiting for app to render...');
		await webDriver.wait(until.elementLocated(By.xpath("//*[contains(text(), 'Content Queue')] | //*[contains(text(), 'Project root not configured')]")), TIMEOUT);

		const title = await webDriver.getTitle();
		console.log(`[e2e:tauri] Window title: ${title}`);

		const nav = await webDriver.findElement(By.css('nav'));
		const navText = await nav.getText();
		if (!navText.includes('Queue') || !navText.includes('Prompt') || !navText.includes('Settings')) {
			throw new Error('Navigation sidebar did not render expected links');
		}

		await webDriver.findElement(By.xpath("//nav//a[contains(., 'Prompt')]")).click();
		await webDriver.wait(until.elementLocated(By.xpath("//*[contains(text(), 'Prompt Builder')]")), TIMEOUT);

		console.log('[e2e:tauri] Smoke test passed.');
	} finally {
		if (webDriver) {
			try {
				await webDriver.quit();
			} catch (e) {
				console.error('[e2e:tauri] Error quitting WebDriver:', e);
			}
		}
		driverProcess.kill();
	}
}

main().catch((e) => {
	console.error('[e2e:tauri] Test failed:', e);
	process.exit(1);
});
