// Dependency-free browser regression driver. Run through the opt-in Go
// integration test, which serves actual schemas and built assets over loopback.
import { spawn } from 'node:child_process';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import assert from 'node:assert/strict';

const [baseURL, executable, screenshot] = process.argv.slice(2);
const profile = await mkdtemp(join(tmpdir(), 'pletka-editor-browser-'));
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
  socket.onmessage = event => {
    const data = JSON.parse(event.data);
    if (data.id) {
      const request = pending.get(data.id); if (!request) return;
      pending.delete(data.id); clearTimeout(request.timer);
      if (data.error) request.reject(new Error(JSON.stringify(data.error))); else request.resolve(data.result);
    } else if (data.method === 'Runtime.exceptionThrown') runtimeErrors.push(data.params.exceptionDetails.text + ' ' + (data.params.exceptionDetails.exception?.description || ''));
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
  await cdp('Runtime.enable'); await cdp('Page.enable');
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
  const fill = async (id, value) => {
    await waitFor(`!!document.getElementById(${JSON.stringify(id)})`);
    await evaluate(`(() => {const input=document.getElementById(${JSON.stringify(id)});input.value=${JSON.stringify(value)};input.dispatchEvent(new Event('input',{bubbles:true}));})()`);
  };
  const api = '/api/v2/ontologies/EDITOR_ONTOLOGY/draft';
  const current = () => evaluate(`fetch(${JSON.stringify(api)}).then(r=>r.json())`);
  await cdp('Page.navigate', { url: baseURL + '/admin/ontologies/EDITOR_ONTOLOGY/draft/page' });
  await click('Edit draft');
  await waitFor('!!document.getElementById("source_0_content")');
  assert.equal(await evaluate('document.body.innerText.includes("Unknown widget")'), false);
  assert.equal(await evaluate('document.querySelector("input[name=revision]").value'), '1');
  await fill('name', 'My browser edit');
  await evaluate(`(async()=>{const d=await fetch(${JSON.stringify(api)}).then(r=>r.json());d.document.ontology.name='Concurrent edit';d.document.ontology.homepage_url='https://remote.example/';const r=await fetch(${JSON.stringify(api)}+'/'+d.draft_id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({revision:d.revision,document:d.document})});if(!r.ok)throw new Error(await r.text());})()`);
  await click('Save draft');
  await waitFor('document.body.innerText.includes("Your unsaved values are still")');
  assert.equal(await evaluate('document.getElementById("name").value'), 'My browser edit');
  await click('Compare with latest');
  await waitFor('!!document.querySelector("input[name=conflict-name][value=mine]")');
  assert.equal(await evaluate('document.querySelector("input[name=revision]").value'), '1');
  await evaluate('document.querySelector("input[name=conflict-name][value=mine]").click()');
  await click('Apply review to form');
  assert.equal(await evaluate('document.querySelector("input[name=revision]").value'), '2');
  assert.equal(await evaluate('document.getElementById("homepage_url").value'), 'https://remote.example/');
  await click('Save draft');
  await waitFor('document.body.innerText.includes("Saved draft") && !document.getElementById("name")');
  let saved = await current();
  assert.equal(saved.revision, 3); assert.equal(saved.document.ontology.name, 'My browser edit');
  assert.equal(saved.document.ontology.homepage_url, 'https://remote.example/');
  const originalSource = saved.document.primary.content;
  // Invalid RDF is a valid draft save, but publication must report its source
  // error even though the publish form has no editable source field.
  await click('Edit draft'); await fill('source_0_content', 'unfinished RDF'); await click('Save draft');
  await waitFor('!document.getElementById("source_0_content") && document.body.innerText.includes("Saved draft")');
  await click('Publish saved draft'); await fill('version', 'browser-1'); await click('Publish');
  await waitFor('document.querySelector("[role=dialog]") !== null'); await click('Publish');
  await waitFor('document.body.innerText.includes("main.rdf:")');
  assert.equal((await current()).publication, undefined);
  await click('Cancel'); await click('Edit draft'); await fill('source_0_content', originalSource); await click('Save draft');
  await waitFor('!document.getElementById("source_0_content") && document.body.innerText.includes("Saved draft")');
  await click('Publish saved draft'); await fill('version', 'browser-1'); await click('Publish');
  await waitFor('document.querySelector("[role=dialog]") !== null'); await click('Publish');
  await waitFor('document.body.innerText.includes("Published as browser-1")');
  saved = await current(); assert.equal(saved.publication.version, 'browser-1');
  assert.equal(await evaluate('Array.from(document.querySelectorAll("button")).some(b => b.textContent.trim()==="Publish saved draft")'), false);
  assert.deepEqual(runtimeErrors, []);
  if (screenshot) {
    const capture = await cdp('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
    await writeFile(screenshot, Buffer.from(capture.data, 'base64'));
  }
  console.log('Browser PASS: actual schema forms, revision conflict review, source validation, publication and receipt UI.');
  await call('Browser.close');
} finally {
  socket?.close();
  try { process.kill(-browser.pid, 'SIGTERM'); } catch {}
  await rm(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
}
