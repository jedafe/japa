import { spawn } from 'node:child_process';

// The native Linux build needs the system pkg-config paths to discover
// GTK/WebKit dependencies. Homebrew's pkg-config can shadow the system one
// and cause the Tauri bundler to fail.
if (process.platform === 'linux' && !process.env.PKG_CONFIG_PATH) {
	process.env.PKG_CONFIG_PATH = '/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig';
}

const tauri = spawn('tauri', ['build'], {
	stdio: 'inherit',
	env: process.env,
	shell: false
});

tauri.on('exit', (code) => {
	process.exit(code ?? 0);
});
