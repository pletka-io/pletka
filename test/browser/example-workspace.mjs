// Dependency-free browser regression driver. Run through the opt-in Go
// integration test, which serves actual schemas and built assets over loopback.
import { spawn } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import assert from 'node:assert/strict';

const [baseURL, executable] = process.argv.slice(2);
const profile = await mkdtemp(join(tmpdir(), 'pletka-example-browser-'));
const browser = spawn(executable, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--no-first-run', '--disable-background-networking', '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], { stdio: ['ignore', 'ignore', 'pipe'], detached: true });
let socket;
try {
  const debugURL = await new Promise((resolve, reject) => {
    let output = '';
    const timer = setTimeout(() => reject(new Error('Chromium did not start')), 20000);
    browser.on('error', error => { clearTimeout(timer); reject(error); });
    browser.stderr.on('data', chunk => {
      output += chunk.toString();
      const match = output.match(/DevTools listening on (ws:\/\/\S+)/);
      if (match) { clearTimeout(timer); resolve(match[1]); }
    });
    browser.on('exit', code => { clearTimeout(timer); reject(new Error(`Chromium exited: ${code}\n${output.slice(-2000)}`)); });
  });
  socket = new WebSocket(debugURL);
  await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
  let sequence = 0;
  const pending = new Map();
  const runtimeErrors = [];
  const requests = [];
  socket.onmessage = event => {
    const data = JSON.parse(event.data);
    if (data.id) {
      const request = pending.get(data.id); if (!request) return;
      pending.delete(data.id); clearTimeout(request.timer);
      if (data.error) request.reject(new Error(JSON.stringify(data.error))); else request.resolve(data.result);
    } else if (data.method === 'Network.requestWillBeSent') requests.push(data.params.request);
    else if (data.method === 'Runtime.exceptionThrown') runtimeErrors.push(data.params.exceptionDetails.text + ' ' + (data.params.exceptionDetails.exception?.description || ''));
  };
  const call = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
    const id = ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error(`CDP timeout: ${method}`)); }, 20000);
    pending.set(id, { resolve, reject, timer });
    socket.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
  });
  const { targetId } = await call('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await call('Target.attachToTarget', { targetId, flatten: true });
  const cdp = (method, params) => call(method, params, sessionId);
  await cdp('Runtime.enable'); await cdp('Page.enable'); await cdp('Network.enable');
  await cdp('Emulation.setDeviceMetricsOverride', { width: 1280, height: 1000, deviceScaleFactor: 1, mobile: false });
  const evaluate = async expression => {
    const result = await cdp('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true, userGesture: true });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
    return result.result.value;
  };
  const waitFor = async expression => {
    const until = Date.now() + 20000;
    while (Date.now() < until) {
      try { if (await evaluate(expression)) return; } catch (error) { if (!/context|navigat/i.test(error.message)) throw error; }
      await new Promise(resolve => setTimeout(resolve, 75));
    }
    throw new Error(`Timed out: ${expression}\n${await evaluate('document.body.innerText')}\nRuntime errors: ${JSON.stringify(runtimeErrors)}`);
  };
  const click = async label => {
    const expression = `Array.from(document.querySelectorAll('button')).filter(b => b.textContent.trim()===${JSON.stringify(label)} && b.getClientRects().length && !b.disabled).at(-1)`;
    await waitFor(`!!(${expression})`); await evaluate(`(${expression}).click()`);
  };
  await cdp('Page.navigate', { url: baseURL + '/__example-ui' });
  await waitFor('document.body.innerText.includes("Recorded content")');
  const buttons = () => evaluate('Array.from(document.querySelectorAll("button")).filter(b=>b.getClientRects().length).map(b=>b.textContent.trim())');
  const text = await evaluate('document.body.innerText');
  assert.match(text, /Viewing Captured target/);
  assert.match(text, /Captured slot/);
  assert.match(text, /Pinned membership label/);
  assert.match(text, /Other recorded values/);
  assert.equal((await buttons()).some(b => b === 'Edit' || /Save|Remove|Add /.test(b)), false);
  assert.equal(await evaluate('!!document.querySelector("input:not([type=hidden]), textarea, select")'), false);
  const reads = requests.filter(r=>r.url.includes('/examples'));
  assert.equal(reads.length, 2, JSON.stringify(reads)); // parent inventory + shared workspace form
  assert.equal(reads.filter(r=>r.url.includes('form-schema')).length, 1);
  for (const request of reads) assert.equal(new URL(request.url).searchParams.get('version'), '2.0.0');
  assert.equal(requests.some(r=>r.method!=='GET'), false);
  assert.equal(requests.some(r=>/vocabulary-entries|entries\/search/.test(r.url)), false);
  await evaluate('document.cookie="test-owner=1; path=/"');
  await cdp('Page.navigate', { url: baseURL + '/__example-ui' });
  await waitFor('document.body.innerText.includes("Editing Captured target")');
  assert.ok((await buttons()).includes('Edit'));
  assert.ok((await buttons()).some(b => /Save/.test(b)));
  await click('Edit');
  await waitFor('Array.from(document.querySelectorAll("input,textarea")).some(e=>e.value==="Recorded content")');
  assert.equal(requests.some(r=>/vocabulary-entries|entries\/search/.test(r.url)), false);
  assert.deepEqual(runtimeErrors, []);
  console.log('Browser PASS: shared workspace, release values/concept labels, no mutation controls or live enrichment, draft editing controls and values.');
  await call('Browser.close');
} finally {
  socket?.close();
  try { process.kill(-browser.pid, 'SIGTERM'); } catch {}
  await rm(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
}
