import { Builder } from 'selenium-webdriver';
import { readFileSync } from 'node:fs';

const binary = '/run/media/jmobile/Japa/studio-target/release/studio-desktop';
const apiKey = readFileSync('/tmp/jc_api_key.txt', 'utf8').trim();

async function main() {
  const driver = await new Builder()
    .usingServer('http://127.0.0.1:4446')
    .withCapabilities({
      browserName: 'wry',
      'tauri:options': { application: binary },
    } as any)
    .build();

  try {
    await driver.sleep(1500);

    const result = await driver.executeScript(async (key: string) => {
      try {
        const res = await fetch('http://localhost:8080/api/v1/auth/exchange-key', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-API-Key': key,
          },
          body: '{}',
        });
        const text = await res.text();
        return { ok: res.ok, status: res.status, statusText: res.statusText, body: text.slice(0, 200) };
      } catch (e: any) {
        return { error: String(e), message: e?.message, name: e?.name };
      }
    }, apiKey);
    console.log('exchange-key result:', result);
  } finally {
    await driver.quit();
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
