import { LitElement, html, css } from 'lit';

// --- small API client ------------------------------------------------------

const api = {
  async json(path, opts = {}) {
    const r = await fetch(path, opts);
    if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
    const ct = r.headers.get('content-type') || '';
    return ct.includes('application/json') ? r.json() : r.text();
  },
  runs:       () => api.json('/api/runs'),
  run:        (id) => api.json(`/api/runs/${encodeURIComponent(id)}`),
  workflows:  () => api.json('/api/workflows'),
  workflow:   (scope, name) => api.json(`/api/workflows/${scope}/${encodeURIComponent(name)}`),
  saveYaml:   (scope, name, yaml) => api.json(`/api/workflows/${scope}/${encodeURIComponent(name)}`, {
                method: 'PUT', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ yaml }) }),
  validate:   (yaml) => api.json('/api/validate', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ yaml }) }),
  remove:     (scope, name) => api.json(`/api/workflows/${scope}/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  duplicate:  (scope, name, toScope, toName) => api.json(`/api/workflows/${scope}/${encodeURIComponent(name)}/duplicate`, {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ toScope, toName }) }),
  move:       (scope, name, toScope, toName) => api.json(`/api/workflows/${scope}/${encodeURIComponent(name)}/move`, {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ toScope, toName }) }),
  startRun:   (body) => api.json('/api/run', {
                method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  stats:      () => api.json('/api/stats'),
};

// --- shared bits -----------------------------------------------------------

function outcomePill(outcome) {
  const cls = outcome === 'complete' ? 'ok'
            : outcome === 'abort' ? 'bad'
            : outcome === 'maxsteps' ? 'warn' : 'run';
  return html`<span class="pill ${cls}">${outcome || 'running'}</span>`;
}

function timeAgo(iso) {
  if (!iso) return '';
  const d = new Date(iso); if (isNaN(d)) return '';
  const s = Math.max(0, (Date.now() - d.getTime()) / 1000);
  if (s < 60) return `${Math.floor(s)}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return d.toISOString().slice(0, 16).replace('T', ' ');
}

// --- root app: routing + layout ------------------------------------------

class KotoApp extends LitElement {
  static properties = {
    view: { state: true },
    params: { state: true },
  };
  createRenderRoot() { return this; } // light DOM so global CSS applies

  constructor() {
    super();
    this.view = 'dashboard';
    this.params = {};
    window.addEventListener('hashchange', () => this.route());
    this.route();
  }
  route() {
    const h = location.hash.replace(/^#\/?/, '') || 'dashboard';
    const [view, ...rest] = h.split('/');
    this.view = view || 'dashboard';
    this.params = { rest };
    // Reset scroll on navigation so each view starts at the top.
    const main = this.querySelector('.main');
    if (main) main.scrollTop = 0;
  }
  nav(href) {
    return (e) => { e.preventDefault(); location.hash = href; };
  }

  render() {
    return html`
      <aside class="nav">
        <div class="brand">koto<span>dashboard</span></div>
        ${[
          ['dashboard', 'Dashboard',  '◇'],
          ['runs',      'Runs',       '⟲'],
          ['live',      'Live',       '○'],
          ['workflows', 'Workflows',  '≡'],
          ['editor',    'Editor',     '✎'],
          ['new',       'New run',    '▷'],
        ].map(([v, label, icon]) => html`
          <a href="#/${v}" @click=${this.nav('/' + v)}
             class="navlink ${this.view === v ? 'active' : ''}">
             <span class="icon">${icon}</span> ${label}
          </a>`)}
        <div class="spacer"></div>
        <div class="hint">koto <code>${KOTO_VERSION}</code></div>
      </aside>
      <main class="main">
        ${this.renderView()}
      </main>
    `;
  }

  renderView() {
    switch (this.view) {
      case 'dashboard': return html`<view-dashboard></view-dashboard>`;
      case 'runs':      return html`<view-runs .params=${this.params}></view-runs>`;
      case 'live':      return html`<view-live .params=${this.params}></view-live>`;
      case 'workflows': return html`<view-workflows></view-workflows>`;
      case 'editor':    return html`<view-editor .params=${this.params}></view-editor>`;
      case 'new':       return html`<view-newrun></view-newrun>`;
      default:          return html`<div class="pad">Unknown view: ${this.view}</div>`;
    }
  }
}
customElements.define('koto-app', KotoApp);

// Embedded version: simple constant; sets via app.js generation could replace.
const KOTO_VERSION = 'dev';

// --- dashboard (stats) ----------------------------------------------------

class ViewDashboard extends LitElement {
  static properties = { stats: { state: true }, err: { state: true } };
  createRenderRoot() { return this; }
  connectedCallback() { super.connectedCallback(); this.load(); }
  async load() {
    try { this.stats = await api.stats(); } catch (e) { this.err = e.message; }
  }
  render() {
    if (this.err) return html`<div class="pad err">${this.err}</div>`;
    const s = this.stats;
    if (!s) return html`<div class="pad dim">loading…</div>`;
    const cards = [
      ['Total runs',       s.total],
      ['Completed',        s.completed, 'ok'],
      ['Aborted',          s.aborted,   'bad'],
      ['Max-steps',        s.maxSteps,  'warn'],
      ['Running',          s.running,   'run'],
      ['Avg steps / run',  s.avgSteps.toFixed(2)],
    ];
    const maxRuns = Math.max(1, ...s.byDay.map(d => d.runs));
    return html`
      <header class="vh"><h1>Dashboard</h1>
        <button @click=${() => this.load()}>refresh</button></header>
      <section class="cards">
        ${cards.map(([k, v, cls]) => html`
          <div class="card">
            <div class="k">${k}</div>
            <div class="v ${cls || ''}">${v}</div>
          </div>`)}
      </section>
      <section class="block">
        <h2>Activity (last 14 days)</h2>
        ${s.byDay.length === 0 ? html`<div class="dim">no runs yet</div>` : html`
          <div class="bars">
            ${s.byDay.map(d => html`
              <div class="bar" title="${d.date} — ${d.runs} runs, ${d.ok} ok">
                <div class="fill" style="height:${(d.runs / maxRuns) * 100}%"></div>
                <div class="ok"   style="height:${(d.ok / maxRuns) * 100}%"></div>
                <div class="lbl">${d.date.slice(5)}</div>
              </div>`)}
          </div>`}
      </section>
      <section class="block">
        <h2>Gate attempts by step</h2>
        ${Object.keys(s.gateAttempts).length === 0
          ? html`<div class="dim">no gate runs yet</div>`
          : html`<table class="t"><thead><tr><th>step</th><th>attempts</th></tr></thead><tbody>
              ${Object.entries(s.gateAttempts).sort((a, b) => b[1] - a[1])
                .map(([k, v]) => html`<tr><td>${k}</td><td>${v}</td></tr>`)}
            </tbody></table>`}
      </section>
    `;
  }
}
customElements.define('view-dashboard', ViewDashboard);

// --- runs list + detail ---------------------------------------------------

class ViewRuns extends LitElement {
  static properties = { runs: { state: true }, sel: { state: true }, detail: { state: true }, err: { state: true } };
  createRenderRoot() { return this; }
  connectedCallback() { super.connectedCallback(); this.refresh(); }
  async refresh() {
    try {
      this.runs = await api.runs();
      const fromHash = (this.params?.rest || [])[0];
      if (fromHash) this.select(fromHash);
    } catch (e) { this.err = e.message; }
  }
  async select(id) {
    this.sel = id;
    try { this.detail = await api.run(id); } catch (e) { this.err = e.message; }
  }
  render() {
    const runs = this.runs || [];
    return html`
      <header class="vh"><h1>Runs</h1>
        <button @click=${() => this.refresh()}>refresh</button></header>
      <section class="split">
        <div class="list">
          ${runs.length === 0 ? html`<div class="pad dim">no runs yet — start one from “New run”</div>` : runs.map(r => html`
            <div class="row ${this.sel === r.id ? 'active' : ''}" @click=${() => this.select(r.id)}>
              <div class="rid">${r.id}</div>
              <div class="rmeta">
                ${outcomePill(r.outcome)}
                <span class="dim">${r.workflow || '?'}</span>
                <span class="dim">· ${r.steps} steps</span>
              </div>
              <div class="rtask">${r.task || ''}</div>
              <div class="rtime dim">${timeAgo(r.started)}</div>
            </div>`)}
        </div>
        <div class="detail">
          ${this.detail
            ? html`<run-detail .data=${this.detail}></run-detail>`
            : html`<div class="pad dim">select a run on the left</div>`}
        </div>
      </section>
    `;
  }
}
customElements.define('view-runs', ViewRuns);

class RunDetail extends LitElement {
  static properties = { data: {} };
  createRenderRoot() { return this; }
  render() {
    const d = this.data;
    if (!d) return html``;
    return html`
      <header class="dh">
        <h2>${d.id} ${outcomePill(d.summary.outcome)}</h2>
        <div class="dim">${d.summary.workflow} · ${d.events.length} events · ${d.path}</div>
        <div class="dim">task: ${d.summary.task || '—'}</div>
      </header>
      <ol class="timeline">
        ${d.events.map(e => html`
          <li class="evt ${e.type}">
            <div class="ehead">
              <span class="etype">${e.type}</span>
              ${e.step ? html`<span class="estep">${e.step}</span>` : ''}
              ${e.message ? html`<span class="emsg dim">${e.message}</span>` : ''}
              <span class="etime dim">${(e.time || '').slice(11,19)}</span>
            </div>
            ${e.detail ? html`<pre>${JSON.stringify(e.detail, null, 2)}</pre>` : ''}
          </li>`)}
      </ol>
    `;
  }
}
customElements.define('run-detail', RunDetail);

// --- live view (SSE) ------------------------------------------------------

class ViewLive extends LitElement {
  static properties = { runId: { state: true }, events: { state: true }, runs: { state: true } };
  createRenderRoot() { return this; }
  connectedCallback() {
    super.connectedCallback();
    this.events = [];
    this.bootstrap();
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    if (this.es) this.es.close();
  }
  async bootstrap() {
    this.runs = await api.runs();
    const hint = (this.params?.rest || [])[0];
    const id = hint || (this.runs[0] && this.runs[0].id);
    if (id) this.attach(id);
  }
  attach(id) {
    if (this.es) this.es.close();
    this.runId = id;
    this.events = [];
    this.es = new EventSource(`/api/runs/${encodeURIComponent(id)}/stream`);
    this.es.onmessage = (m) => {
      try {
        const ev = JSON.parse(m.data);
        this.events = [...this.events, ev];
        // Close the connection once the run ends so the EventSource does not
        // auto-reconnect and replay the file.
        if (ev.type === 'run_end' && this.es) {
          this.es.close();
          this.es = null;
        }
      } catch { /* ignore non-JSON */ }
      this.requestUpdate();
    };
    this.es.onerror = () => { /* server closes when run_end seen */ };
  }
  render() {
    return html`
      <header class="vh"><h1>Live</h1>
        <select @change=${(e) => this.attach(e.target.value)}>
          ${(this.runs || []).map(r => html`<option value=${r.id} ?selected=${r.id === this.runId}>${r.id} — ${r.workflow || ''}</option>`)}
        </select></header>
      <div class="livebox">
        ${this.events.length === 0 ? html`<div class="dim">waiting for events…</div>` : ''}
        ${this.events.map(e => html`
          <div class="lline">
            <span class="etime dim">${(e.time || '').slice(11,19)}</span>
            <span class="etype">${e.type}</span>
            <span class="estep">${e.step || ''}</span>
            <span class="dim">${e.message || ''}</span>
          </div>`)}
      </div>
    `;
  }
}
customElements.define('view-live', ViewLive);

// --- workflows list -------------------------------------------------------

class ViewWorkflows extends LitElement {
  static properties = { items: { state: true }, err: { state: true } };
  createRenderRoot() { return this; }
  connectedCallback() { super.connectedCallback(); this.load(); }
  async load() {
    try { this.items = await api.workflows(); } catch (e) { this.err = e.message; }
  }
  edit(scope, name) { location.hash = `#/editor/${scope}/${name}`; }
  async duplicate(it) {
    const toName = prompt('New name:', it.name + '-copy');
    if (!toName) return;
    const toScope = it.source === 'builtin' ? 'local' : it.source;
    await api.duplicate(it.source, it.name, toScope, toName);
    this.load();
  }
  async move(it) {
    if (it.source === 'builtin') { alert('cannot move a builtin'); return; }
    const target = prompt('Move to scope (local / user):', it.source === 'local' ? 'user' : 'local');
    if (!target || target === it.source) return;
    await api.move(it.source, it.name, target, it.name);
    this.load();
  }
  async remove(it) {
    if (it.source === 'builtin') { alert('cannot delete a builtin'); return; }
    if (!confirm(`Delete workflow "${it.name}" from ${it.source}?`)) return;
    await api.remove(it.source, it.name);
    this.load();
  }
  newWorkflow() {
    const name = prompt('Workflow name:');
    if (!name) return;
    location.hash = `#/editor/local/${name}`;
  }
  render() {
    const items = this.items || [];
    return html`
      <header class="vh"><h1>Workflows</h1>
        <button class="primary" @click=${() => this.newWorkflow()}>+ New workflow</button>
        <button @click=${() => this.load()}>refresh</button></header>
      ${this.err ? html`<div class="pad err">${this.err}</div>` : ''}
      <table class="t">
        <thead><tr><th>name</th><th>source</th><th>path</th><th></th></tr></thead>
        <tbody>
          ${items.map(it => html`
            <tr>
              <td><a href="#" @click=${(e) => { e.preventDefault(); this.edit(it.source, it.name); }}>${it.name}</a></td>
              <td><span class="pill ${it.source === 'builtin' ? 'warn' : 'ok'}">${it.source}</span></td>
              <td class="dim">${it.path || '(embedded)'}</td>
              <td class="actions">
                <button @click=${() => this.edit(it.source, it.name)}>${it.source === 'builtin' ? 'view' : 'edit'}</button>
                <button @click=${() => this.duplicate(it)}>duplicate</button>
                ${it.source !== 'builtin' ? html`
                  <button @click=${() => this.move(it)}>move</button>
                  <button class="danger" @click=${() => this.remove(it)}>delete</button>` : ''}
              </td>
            </tr>`)}
        </tbody>
      </table>
    `;
  }
}
customElements.define('view-workflows', ViewWorkflows);

// --- workflow editor ------------------------------------------------------

class ViewEditor extends LitElement {
  static properties = {
    scope: { state: true }, name: { state: true },
    yaml: { state: true }, readOnly: { state: true },
    parsed: { state: true }, vMsg: { state: true }, vOK: { state: true }, saved: { state: true },
  };
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [scope = 'local', name] = this.params?.rest || [];
    this.scope = scope; this.name = name; this.saved = false;
    if (!name) return;
    try {
      const r = await api.workflow(scope, name);
      this.yaml = r.yaml; this.readOnly = !!r.readOnly;
    } catch (e) {
      // new file with default template
      this.yaml = defaultWorkflowYaml(name);
      this.readOnly = false;
    }
    this.parse();
  }

  async parse() {
    try {
      const r = await api.validate(this.yaml);
      this.vOK = r.ok; this.vMsg = r.ok ? `valid · ${r.steps} steps · initial ${r.initial}` : r.error;
      if (r.ok) this.parsed = parseYamlForCanvas(this.yaml);
    } catch (e) { this.vOK = false; this.vMsg = e.message; }
  }

  async save() {
    if (this.readOnly) { alert('builtin workflows are read-only — duplicate to edit'); return; }
    try {
      await api.saveYaml(this.scope, this.name, this.yaml);
      this.saved = true; setTimeout(() => { this.saved = false; this.requestUpdate(); }, 1500);
    } catch (e) { alert(e.message); }
  }

  // Move a step up or down in the YAML and re-render. Operates on the parsed
  // canvas view; the YAML is then regenerated from a structured form (round-trip
  // via the simple parser keeps comments, but we re-emit cleanly).
  moveStep(i, dir) {
    const steps = this.parsed?.steps || [];
    const j = i + dir;
    if (j < 0 || j >= steps.length) return;
    [steps[i], steps[j]] = [steps[j], steps[i]];
    this.yaml = emitYaml(this.parsed);
    this.parse();
  }

  patchStep(i, key, val) {
    if (!this.parsed) return;
    this.parsed.steps[i][key] = val;
    this.yaml = emitYaml(this.parsed);
    this.parse();
  }

  addStep() {
    if (!this.parsed) return;
    this.parsed.steps.push({ name: `step${this.parsed.steps.length + 1}`, type: 'agent', edit: false, persona: 'do something\nWhen done, end with __NEXT:COMPLETE__', rules: [{ on: '__NEXT:COMPLETE__', to: 'COMPLETE' }] });
    this.yaml = emitYaml(this.parsed);
    this.parse();
  }

  removeStep(i) {
    if (!confirm('delete this step?')) return;
    this.parsed.steps.splice(i, 1);
    this.yaml = emitYaml(this.parsed);
    this.parse();
  }

  onDragStart(i) { return (e) => { e.dataTransfer.setData('text/plain', String(i)); }; }
  onDragOver(e) { e.preventDefault(); }
  onDrop(toIdx) {
    return (e) => {
      e.preventDefault();
      const from = parseInt(e.dataTransfer.getData('text/plain'), 10);
      if (isNaN(from) || from === toIdx) return;
      const steps = this.parsed.steps;
      const [m] = steps.splice(from, 1);
      steps.splice(toIdx, 0, m);
      this.yaml = emitYaml(this.parsed);
      this.parse();
    };
  }

  render() {
    if (!this.name) return html`<div class="pad">no workflow selected</div>`;
    const steps = this.parsed?.steps || [];
    const stepNames = steps.map(s => s.name);
    return html`
      <header class="vh">
        <h1>${this.name} <span class="pill ${this.scope === 'builtin' ? 'warn' : 'ok'}">${this.scope}</span></h1>
        ${this.readOnly ? html`<span class="dim">read-only</span>` : html`
          <button class="primary" @click=${() => this.save()}>${this.saved ? '✓ saved' : 'save'}</button>`}
        <button @click=${() => this.addStep()}>+ add step</button>
        <span class="dim">${this.vOK ? '✓' : '✗'} ${this.vMsg || ''}</span>
      </header>
      <div class="split">
        <div class="canvas">
          ${steps.map((st, i) => html`
            <div class="stepcard ${st.type}"
                 draggable="true"
                 @dragstart=${this.onDragStart(i)}
                 @dragover=${this.onDragOver}
                 @drop=${this.onDrop(i)}>
              <div class="schead">
                <span class="pill">${st.type}</span>
                <input class="sname" .value=${st.name} @change=${(e) => this.patchStep(i, 'name', e.target.value)} ?disabled=${this.readOnly}>
                <div class="spacer"></div>
                <button @click=${() => this.moveStep(i, -1)} ?disabled=${this.readOnly || i === 0}>↑</button>
                <button @click=${() => this.moveStep(i, +1)} ?disabled=${this.readOnly || i === steps.length - 1}>↓</button>
                <button class="danger" @click=${() => this.removeStep(i)} ?disabled=${this.readOnly}>delete</button>
              </div>
              ${st.type === 'agent' ? html`
                <label>persona<textarea rows="6" ?readonly=${this.readOnly}
                  .value=${st.persona || ''}
                  @change=${(e) => this.patchStep(i, 'persona', e.target.value)}></textarea></label>
                <label class="row">
                  <input type="checkbox" .checked=${!!st.edit} ?disabled=${this.readOnly}
                    @change=${(e) => this.patchStep(i, 'edit', e.target.checked)}>
                  allow file edits
                </label>
                <div class="rules">
                  <div class="dim small">rules (marker → next step)</div>
                  ${(st.rules || []).map((rl, ri) => {
                    // Build a stable set of destination options that always includes
                    // the current value, even if it's not a known step name.
                    const opts = Array.from(new Set([
                      ...(rl.to ? [rl.to] : []),
                      'COMPLETE', 'ABORT',
                      ...stepNames.filter(n => n !== st.name),
                    ]));
                    return html`
                    <div class="rule">
                      <input .value=${rl.on || ''} placeholder="__NEXT:test__"
                        @change=${(e) => { st.rules[ri].on = e.target.value; this.yaml = emitYaml(this.parsed); this.parse(); }}>
                      <span>→</span>
                      <select @change=${(e) => { st.rules[ri].to = e.target.value; this.yaml = emitYaml(this.parsed); this.parse(); }} ?disabled=${this.readOnly}>
                        ${opts.map(n => html`<option value=${n} ?selected=${rl.to === n}>${n}</option>`)}
                      </select>
                      <button class="danger" @click=${() => { st.rules.splice(ri, 1); this.yaml = emitYaml(this.parsed); this.parse(); }} ?disabled=${this.readOnly}>×</button>
                    </div>`;
                  })}
                  <button @click=${() => { (st.rules ||= []).push({ on: '__NEXT:next__', to: 'COMPLETE' }); this.yaml = emitYaml(this.parsed); this.parse(); }} ?disabled=${this.readOnly}>+ rule</button>
                </div>
              ` : st.type === 'gate' ? html`
                <label>run (shell command)<input .value=${st.run || ''} @change=${(e) => this.patchStep(i, 'run', e.target.value)} ?disabled=${this.readOnly}></label>
                <div class="grid2">
                  <label>max_retries<input type="number" .value=${String(st.max_retries ?? 3)} @change=${(e) => this.patchStep(i, 'max_retries', parseInt(e.target.value, 10) || 0)} ?disabled=${this.readOnly}></label>
                  ${(() => {
                    const passOpts = Array.from(new Set([...(st.on_pass ? [st.on_pass] : []), 'COMPLETE','ABORT',...stepNames.filter(n=>n!==st.name)]));
                    const failOpts = Array.from(new Set([...(st.on_fail ? [st.on_fail] : []), 'COMPLETE','ABORT',...stepNames.filter(n=>n!==st.name)]));
                    return html`
                      <label>on_pass<select @change=${(e) => this.patchStep(i, 'on_pass', e.target.value)} ?disabled=${this.readOnly}>
                        ${passOpts.map(n=>html`<option value=${n} ?selected=${st.on_pass===n}>${n}</option>`)}
                      </select></label>
                      <label>on_fail<select @change=${(e) => this.patchStep(i, 'on_fail', e.target.value)} ?disabled=${this.readOnly}>
                        ${failOpts.map(n=>html`<option value=${n} ?selected=${st.on_fail===n}>${n}</option>`)}
                      </select></label>`;
                  })()}
                </div>
              ` : html`<label>prompt<textarea rows="3" .value=${st.prompt || ''} @change=${(e) => this.patchStep(i, 'prompt', e.target.value)} ?disabled=${this.readOnly}></textarea></label>`}
            </div>`)}
        </div>
        <div class="yamlpane">
          <div class="yhead">YAML <span class="dim">(editable; saved on click)</span></div>
          <textarea class="ymono" ?readonly=${this.readOnly}
            .value=${this.yaml || ''}
            @input=${(e) => { this.yaml = e.target.value; }}
            @change=${() => this.parse()}></textarea>
        </div>
      </div>
    `;
  }
}
customElements.define('view-editor', ViewEditor);

// --- new run --------------------------------------------------------------

class ViewNewRun extends LitElement {
  static properties = { wfs: { state: true }, started: { state: true }, err: { state: true } };
  createRenderRoot() { return this; }
  connectedCallback() { super.connectedCallback(); api.workflows().then(x => this.wfs = x); }
  async submit(e) {
    e.preventDefault();
    const f = e.target;
    const body = {
      workflow: f.workflow.value,
      task:     f.task.value,
      provider: f.provider.value || undefined,
      model:    f.model.value || undefined,
      vars:     parseSet(f.vars.value),
      dryRun:   f.dryRun.checked,
      noIsolate: f.noIsolate.checked,
    };
    try {
      const r = await api.startRun(body);
      this.started = r;
      // jump to Live attached to this run
      location.hash = `#/live/${r.id}`;
    } catch (e) { this.err = e.message; }
  }
  render() {
    return html`
      <header class="vh"><h1>New run</h1></header>
      <form class="form" @submit=${this.submit.bind(this)}>
        <label>workflow
          <select name="workflow">
            ${(this.wfs || []).map(w => html`<option value=${w.name}>${w.name} (${w.source})</option>`)}
          </select></label>
        <label>task<textarea name="task" rows="4" required placeholder="describe the change to make"></textarea></label>
        <div class="grid2">
          <label>provider<input name="provider" placeholder="(default from config)"></label>
          <label>model<input name="model" placeholder="(default)"></label>
        </div>
        <label>vars (one per line: KEY=value)<textarea name="vars" rows="3" placeholder="test_cmd=go test ./..."></textarea></label>
        <label class="row"><input type="checkbox" name="dryRun"> dry run</label>
        <label class="row"><input type="checkbox" name="noIsolate"> --no-isolate (let agent read host config)</label>
        <button class="primary" type="submit">Start run</button>
        ${this.err ? html`<div class="err">${this.err}</div>` : ''}
        ${this.started ? html`<div class="dim">started ${this.started.id}</div>` : ''}
      </form>
    `;
  }
}
customElements.define('view-newrun', ViewNewRun);

// --- helpers: tiny YAML round-trip for the canvas editor ------------------

// parseYamlForCanvas reuses a minimal subset parser sufficient for the koto
// workflow schema. The server is the source of truth for validation; this is
// only for round-tripping the structured editor view.
function parseYamlForCanvas(yaml) {
  // Light parser: we drop comments, then track key:value and lists by indentation.
  // Good enough for our schema since the editor regenerates from this object.
  const lines = yaml.split('\n').map(l => l.replace(/\t/g, '  '));
  const root = { name: '', initial: '', max_steps: 20, vars: {}, steps: [] };
  let cur = null;
  let inVars = false;
  let inStepsList = false;
  let buffer = null; // for multiline block scalar (persona | / >)
  let bufferKey = null;
  let bufferIndent = 0;
  for (let li = 0; li < lines.length; li++) {
    const ln = lines[li];
    // multiline scalar handling
    if (buffer !== null) {
      const m = ln.match(/^(\s*)(.*)$/);
      if (ln.trim() === '' || (m && m[1].length >= bufferIndent + 2)) {
        buffer.push(ln.slice(bufferIndent + 2));
        continue;
      }
      cur[bufferKey] = buffer.join('\n').replace(/\n+$/, '');
      buffer = null; bufferKey = null;
    }
    if (/^\s*#/.test(ln) || ln.trim() === '') continue;

    const top = ln.match(/^([a-zA-Z_][a-zA-Z0-9_]*):\s*(.*)$/);
    if (top && !ln.startsWith(' ')) {
      const [, k, v] = top;
      if (k === 'vars')    { inVars = true; inStepsList = false; cur = null; continue; }
      if (k === 'steps')   { inVars = false; inStepsList = true; cur = null; continue; }
      inVars = false; inStepsList = false; cur = null;
      if (k === 'max_steps') root.max_steps = parseInt(v, 10) || 20;
      else root[k] = stripQuotes(v);
      continue;
    }

    // a step starts with `- name: …`
    if (inStepsList && /^\s*- name:/.test(ln)) {
      cur = {};
      root.steps.push(cur);
      const m = ln.match(/^\s*- name:\s*(.*)$/);
      cur.name = stripQuotes(m[1]);
      continue;
    }

    if (inVars) {
      const m = ln.match(/^\s+([a-zA-Z_][a-zA-Z0-9_]*):\s*(.*)$/);
      if (m) root.vars[m[1]] = stripQuotes(m[2]);
      continue;
    }

    if (cur) {
      // Rule list items take priority — they share the `to:` key name with the
      // step level and would otherwise be captured as a step field.
      const r1 = ln.match(/^\s+- on:\s*(.*)$/);
      if (r1) { (cur.rules ||= []).push({ on: stripQuotes(r1[1]), to: '' }); continue; }
      // `to:` only belongs to the latest rule when we are currently in a rules block.
      const r2 = ln.match(/^(\s+)to:\s*(.*)$/);
      if (r2 && cur.rules && cur.rules.length) {
        const indent = r2[1].length;
        // rule indentation is deeper than step-level keys (rules entries start at
        // step_indent + 4 spaces, the `to:` line is at +8). step keys are at +4.
        if (indent >= 8) {
          cur.rules[cur.rules.length - 1].to = stripQuotes(r2[2]);
          continue;
        }
      }
      const m = ln.match(/^\s+([a-zA-Z_][a-zA-Z0-9_]*):\s*(.*)$/);
      if (m) {
        const [, k, v] = m;
        if (v === '|' || v === '>' || v === '|-' || v === '>-') {
          buffer = []; bufferKey = k; bufferIndent = (ln.match(/^\s*/)[0].length);
          continue;
        }
        if (k === 'rules')        { cur.rules = []; continue; }
        if (k === 'max_retries')  { cur.max_retries = parseInt(v, 10) || 0; continue; }
        if (k === 'edit')         { cur.edit = (v === 'true'); continue; }
        cur[k] = stripQuotes(v);
        continue;
      }
    }
  }
  if (buffer !== null && cur && bufferKey) {
    cur[bufferKey] = buffer.join('\n').replace(/\n+$/, '');
  }
  return root;
}

function stripQuotes(s) {
  s = s.trim();
  if (s.startsWith('"') && s.endsWith('"')) return s.slice(1, -1);
  if (s.startsWith("'") && s.endsWith("'")) return s.slice(1, -1);
  return s;
}

function emitYaml(o) {
  const lines = [`name: ${o.name || 'new'}`, `initial: ${o.initial || (o.steps[0] && o.steps[0].name) || 'first'}`, `max_steps: ${o.max_steps || 20}`];
  if (o.vars && Object.keys(o.vars).length) {
    lines.push('vars:');
    for (const [k, v] of Object.entries(o.vars)) lines.push(`  ${k}: ${JSON.stringify(String(v))}`);
  }
  lines.push('', 'steps:');
  for (const s of o.steps) {
    lines.push(`  - name: ${s.name}`);
    lines.push(`    type: ${s.type}`);
    if (s.type === 'agent') {
      if (s.edit) lines.push('    edit: true');
      if (s.persona) {
        lines.push('    persona: |');
        for (const pl of String(s.persona).split('\n')) lines.push('      ' + pl);
      }
      if (s.rules && s.rules.length) {
        lines.push('    rules:');
        for (const r of s.rules) {
          lines.push(`      - on: ${JSON.stringify(String(r.on || ''))}`);
          lines.push(`        to: ${r.to || 'COMPLETE'}`);
        }
      }
    } else if (s.type === 'gate') {
      lines.push(`    run: ${JSON.stringify(String(s.run || ''))}`);
      lines.push(`    max_retries: ${s.max_retries ?? 3}`);
      lines.push(`    on_pass: ${s.on_pass || 'COMPLETE'}`);
      lines.push(`    on_fail: ${s.on_fail || 'COMPLETE'}`);
    } else if (s.type === 'approve') {
      if (s.prompt) lines.push(`    prompt: ${JSON.stringify(String(s.prompt))}`);
      if (s.rules && s.rules.length) {
        lines.push('    rules:');
        for (const r of s.rules) {
          lines.push(`      - on: ${JSON.stringify(String(r.on || ''))}`);
          lines.push(`        to: ${r.to || 'COMPLETE'}`);
        }
      }
    }
    lines.push('');
  }
  return lines.join('\n');
}

function defaultWorkflowYaml(name) {
  return `name: ${name}
initial: implement
max_steps: 20
vars:
  test_cmd: "go test ./..."

steps:
  - name: implement
    type: agent
    edit: true
    persona: |
      Implement the task by editing the code.
      Task: {{task}}
      When done, end with __NEXT:gate__
    rules:
      - on: "__NEXT:gate__"
        to: gate

  - name: gate
    type: gate
    run: "{{vars.test_cmd}}"
    max_retries: 5
    on_pass: COMPLETE
    on_fail: fix

  - name: fix
    type: agent
    edit: true
    persona: |
      The gate failed. Fix it. Do not weaken the checks.
      Output:
      {{gate_output}}
      When done, end with __NEXT:gate__
    rules:
      - on: "__NEXT:gate__"
        to: gate
`;
}

function parseSet(text) {
  const out = {};
  for (const line of (text || '').split('\n')) {
    const m = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/);
    if (m) out[m[1]] = m[2];
  }
  return out;
}
