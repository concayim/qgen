'use strict';

const $ = (id) => document.getElementById(id);

const els = {
  kbRoot: $('kbRoot'), maxDocs: $('maxDocs'),
  baseUrl: $('baseUrl'), apiKey: $('apiKey'), model: $('model'), temperature: $('temperature'),
  nInfo: $('nInfo'), nGuidance: $('nGuidance'), nDraft: $('nDraft'), concurrency: $('concurrency'),
  outPath: $('outPath'),
  modelBadge: $('modelBadge'),
  statusText: $('statusText'), counter: $('counter'), progressBar: $('progressBar'),
  resultBar: $('resultBar'), resultText: $('resultText'),
  log: $('log'),
  btnPreview: $('btnPreview'), btnGenerate: $('btnGenerate'), btnCancel: $('btnCancel'),
  btnOpen: $('btnOpen'), btnReveal: $('btnReveal'), btnClearLog: $('btnClearLog'),
  pickRoot: $('pickRoot'), pickOut: $('pickOut'), toggleKey: $('toggleKey'),
};

let lastOutput = '';
let running = false;

// ---- 设置读写 ----
function collectSettings() {
  return {
    baseUrl: els.baseUrl.value.trim(),
    apiKey: els.apiKey.value,
    model: els.model.value.trim(),
    temperature: Number(els.temperature.value) || 0,
    kbRoot: els.kbRoot.value.trim(),
    outPath: els.outPath.value.trim(),
    intents: {
      info: Number(els.nInfo.value) || 0,
      guidance: Number(els.nGuidance.value) || 0,
      draft: Number(els.nDraft.value) || 0,
    },
    concurrency: Number(els.concurrency.value) || 1,
    maxDocs: Number(els.maxDocs.value) || 0,
  };
}

function applySettings(s) {
  els.baseUrl.value = s.baseUrl || '';
  els.apiKey.value = s.apiKey || '';
  els.model.value = s.model || '';
  els.temperature.value = s.temperature ?? 0;
  els.kbRoot.value = s.kbRoot || '';
  els.outPath.value = s.outPath || '';
  els.nInfo.value = s.intents?.info ?? 3;
  els.nGuidance.value = s.intents?.guidance ?? 1;
  els.nDraft.value = s.intents?.draft ?? 1;
  els.concurrency.value = s.concurrency ?? 6;
  els.maxDocs.value = s.maxDocs ?? 0;
  els.modelBadge.textContent = s.model || 'model';
}

async function persist() {
  await window.qgen.saveSettings(collectSettings());
}

// ---- 日志 ----
function log(msg, cls) {
  const span = document.createElement('span');
  if (cls) span.className = cls;
  span.textContent = msg + '\n';
  els.log.appendChild(span);
  els.log.scrollTop = els.log.scrollHeight;
}

function setStatus(text) { els.statusText.textContent = text; }
function setProgress(done, total) {
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  els.progressBar.style.width = pct + '%';
  els.counter.textContent = total > 0 ? `${done}/${total}` : '';
}

function setRunning(on) {
  running = on;
  els.btnGenerate.disabled = on;
  els.btnPreview.disabled = on;
  els.btnCancel.disabled = !on;
}

// ---- 事件流 ----
window.qgen.onEvent((ev) => {
  switch (ev.type) {
    case 'scan':
      setStatus(`已扫描 ${ev.total} 篇文档`);
      setProgress(0, ev.total);
      log(`📚 共扫描到 ${ev.total} 篇文档`, 'dim');
      if (Array.isArray(ev.doc_list)) {
        ev.doc_list.forEach((d, i) => log(`  ${String(i + 1).padStart(3)}. ${d.title}`, 'dim'));
      }
      break;
    case 'start':
      setStatus('生成中…');
      log(`🤖 开始生成（模型 ${ev.model}，每篇 ${ev.total_per_doc} 条 [${ev.mix}]，并发 ${ev.concurrency}）`);
      break;
    case 'progress':
      setProgress(ev.done, ev.total);
      log(`  [${ev.done}/${ev.total}] ${ev.title} -> ${ev.count} 条`);
      break;
    case 'warn':
      log('⚠️ ' + ev.message, 'warn');
      break;
    case 'done':
      if (ev.dry_run) {
        setStatus(`预览完成：${ev.docs} 篇`);
        log(`✅ 预览完成，共 ${ev.docs} 篇文档（未调用模型）`, 'ok');
      } else {
        setStatus('完成');
        setProgress(1, 1);
        lastOutput = ev.output || '';
        els.resultText.textContent = `✅ 共生成 ${ev.questions} 条问题`;
        els.resultBar.hidden = false;
        log(`✅ 完成：共生成 ${ev.questions} 条，已写入 ${ev.output}`, 'ok');
      }
      break;
    case 'error':
      setStatus('出错');
      log('❌ ' + ev.message, 'err');
      break;
    case 'log':
      if (ev.message) log(ev.message, 'dim');
      break;
    case 'exit':
      setRunning(false);
      break;
  }
});

// ---- 运行 ----
function validate(opts, requireModel) {
  if (!opts.kbRoot) { log('❌ 请先选择知识库目录', 'err'); return false; }
  if (requireModel) {
    if (!opts.apiKey) { log('❌ 请填写 API Key', 'err'); return false; }
    if (!opts.model) { log('❌ 请填写模型名', 'err'); return false; }
    if (!opts.outPath) { log('❌ 请选择导出文件路径', 'err'); return false; }
  }
  return true;
}

async function start(dryRun) {
  if (running) return;
  const opts = collectSettings();
  if (!validate(opts, !dryRun)) return;
  await persist();
  els.resultBar.hidden = true;
  setProgress(0, 0);
  setRunning(true);
  setStatus(dryRun ? '扫描中…' : '准备中…');
  const res = await window.qgen.run(Object.assign({}, opts, { dryRun }));
  setRunning(false);
  if (res && !res.ok && res.error) {
    log('❌ ' + res.error, 'err');
    setStatus('出错');
  }
}

els.btnPreview.addEventListener('click', () => start(true));
els.btnGenerate.addEventListener('click', () => start(false));
els.btnCancel.addEventListener('click', async () => {
  await window.qgen.cancel();
  log('⏹ 已请求停止', 'warn');
  setStatus('已停止');
});

els.pickRoot.addEventListener('click', async () => {
  const p = await window.qgen.pickFolder();
  if (p) { els.kbRoot.value = p; persist(); }
});

els.pickOut.addEventListener('click', async () => {
  const p = await window.qgen.pickSave('知识库问题清单.xlsx');
  if (p) { els.outPath.value = p; persist(); }
});

els.toggleKey.addEventListener('click', () => {
  els.apiKey.type = els.apiKey.type === 'password' ? 'text' : 'password';
});

els.btnOpen.addEventListener('click', () => { if (lastOutput) window.qgen.openFile(lastOutput); });
els.btnReveal.addEventListener('click', () => { if (lastOutput) window.qgen.revealFile(lastOutput); });
els.btnClearLog.addEventListener('click', () => { els.log.textContent = ''; });

els.model.addEventListener('input', () => { els.modelBadge.textContent = els.model.value || 'model'; });

['change', 'blur'].forEach((evt) => {
  [els.baseUrl, els.apiKey, els.model, els.temperature, els.maxDocs,
   els.nInfo, els.nGuidance, els.nDraft, els.concurrency, els.kbRoot, els.outPath]
    .forEach((el) => el.addEventListener(evt, persist));
});

// ---- 初始化 ----
(async () => {
  const s = await window.qgen.loadSettings();
  applySettings(s);
  log('就绪。请选择知识库目录与模型配置后开始。', 'dim');
})();
