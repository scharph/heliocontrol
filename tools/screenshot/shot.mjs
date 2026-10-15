// Captures screenshots of the running UI through the Chrome DevTools protocol.
//   node tools/screenshot/shot.mjs <chrome.exe> <outDir> <baseUrl> name=/path[@WxH] ...
// Clicks can be scripted per shot: name=/path@1400x900!css-selector
import { spawn } from 'node:child_process';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const [chrome, outDir, base, ...shots] = process.argv.slice(2);
const port = 9300 + Math.floor(Math.random() * 500);
const profile = mkdtempSync(join(tmpdir(), 'hc-shot-'));
const proc = spawn(chrome, ['--headless=new', '--disable-gpu', '--hide-scrollbars', `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, 'about:blank'], { stdio: 'ignore' });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

let target;
for (let i = 0; i < 50 && !target; i++) {
  await sleep(200);
  try {
    target = (await (await fetch(`http://127.0.0.1:${port}/json`)).json()).find((t) => t.type === 'page');
  } catch {}
}
if (!target) throw new Error('chrome did not start');
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => ws.addEventListener('open', r));
let id = 0;
const pending = new Map();
ws.addEventListener('message', (e) => {
  const m = JSON.parse(e.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.exceptionThrown") console.log("EXCEPTION", JSON.stringify(m.params.exceptionDetails).slice(0, 800));
  if (m.method === "Runtime.consoleAPICalled" && ["error", "warning"].includes(m.params.type))
    console.log("CONSOLE", m.params.type, m.params.args.map((a) => a.value ?? a.description).join(" ").slice(0, 800));
});
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
await send('Page.enable');
await send('Runtime.enable');
for (const spec of shots) {
  const [name, rest] = spec.split('=');
  const [pathAndSize, click] = rest.split('!');
  const [path, size = '1400x1000'] = pathAndSize.split('@');
  const [w, h] = size.split('x').map(Number);
  await send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: w < 600 });
  const nav = await send('Page.navigate', { url: base + path });
  if (nav.error || nav.result?.errorText) console.log('navigate', JSON.stringify(nav));
  await sleep(3500);
  if (click) {
    await send('Runtime.evaluate', { expression: `document.querySelector(${JSON.stringify(click)})?.click()` });
    await sleep(2500);
  }
  const dbg = await send('Runtime.evaluate', { expression: 'location.href + " | " + document.querySelector("app-root")?.innerHTML.length + " | " + document.body.getBoundingClientRect().height', returnByValue: true });
  console.log("dom", dbg.result?.result?.value);
  const r = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
  writeFileSync(join(outDir, name + '.png'), Buffer.from(r.result.data, 'base64'));
  console.log('saved', name);
}
ws.close();
proc.kill();
