package api

// docsHTML is the self-hosted API docs page — dependency-free HTML/CSS/JS that fetches and renders
// /v1/openapi.json at load time, so it's always up to date without a rebuild. No CDN, no external
// scripts — it works on a loopback server with no internet access.
const docsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Virta local API</title>
  <style>
    :root{--bg:#0e0f12;--bg1:#15171c;--bg2:#1c1f26;--line:#262a33;--t0:#e8eaf0;--t1:#9aa1ae;--t2:#5c6370;--acc:#5b8cff;--ok:#3fb950;--warn:#d29922;--danger:#f85149;--mono:"Geist Mono Variable",ui-monospace,monospace}
    *{box-sizing:border-box;margin:0}
    body{background:var(--bg);color:var(--t0);font-family:ui-sans-serif,system-ui,-apple-system,sans-serif;font-size:14px;line-height:1.6;display:flex;min-height:100vh}
    #nav{width:220px;flex:none;border-right:1px solid var(--line);padding:24px 0;overflow-y:auto;position:sticky;top:0;height:100vh;background:var(--bg1)}
    #nav h2{font-size:11px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:var(--t2);padding:0 16px 8px}
    #nav ul{list-style:none;padding:0}
    #nav li a{display:block;padding:5px 16px;color:var(--t1);text-decoration:none;font-size:13px;border-left:2px solid transparent}
    #nav li a:hover{color:var(--t0);background:var(--bg2)}
    #nav li a.active{color:var(--acc);border-left-color:var(--acc)}
    main{flex:1;padding:32px 40px;max-width:900px}
    h1{font-size:24px;font-weight:700;margin-bottom:4px}
    .subtitle{color:var(--t1);margin-bottom:32px;font-size:14px}
    .section{margin-bottom:48px}
    .section h2{font-size:18px;font-weight:600;margin-bottom:16px;padding-bottom:8px;border-bottom:1px solid var(--line)}
    .endpoint{border:1px solid var(--line);border-radius:8px;margin-bottom:12px;overflow:hidden;background:var(--bg1)}
    .endpoint-head{display:flex;align-items:center;gap:12px;padding:12px 16px;cursor:pointer;user-select:none}
    .endpoint-head:hover{background:var(--bg2)}
    .method{font-family:var(--mono);font-size:11px;font-weight:700;padding:2px 8px;border-radius:4px;flex:none}
    .GET{background:color-mix(in srgb,var(--ok) 18%,transparent);color:var(--ok)}
    .POST{background:color-mix(in srgb,var(--acc) 18%,transparent);color:var(--acc)}
    .PUT{background:color-mix(in srgb,var(--warn) 18%,transparent);color:var(--warn)}
    .DELETE{background:color-mix(in srgb,var(--danger) 18%,transparent);color:var(--danger)}
    .path{font-family:var(--mono);font-size:13px;color:var(--t0)}
    .summary{flex:1;color:var(--t1);font-size:13px}
    .scope-badge{font-size:10px;font-weight:700;padding:1px 6px;border-radius:4px;background:var(--bg2);color:var(--t2);border:1px solid var(--line)}
    .try-btn{margin-left:auto;font-size:11px;font-weight:600;padding:4px 10px;border:1px solid var(--line);border-radius:4px;background:none;color:var(--t1);cursor:pointer}
    .try-btn:hover{border-color:var(--acc);color:var(--acc)}
    .endpoint-body{display:none;padding:16px;border-top:1px solid var(--line);background:var(--bg)}
    .endpoint-body.open{display:block}
    .try-area{display:flex;flex-direction:column;gap:8px}
    .try-area label{font-size:12px;color:var(--t2);font-weight:600}
    .try-area input,textarea{width:100%;padding:8px 10px;background:var(--bg1);border:1px solid var(--line);border-radius:6px;color:var(--t0);font-family:var(--mono);font-size:12px;resize:vertical}
    .try-area input:focus,textarea:focus{outline:2px solid var(--acc);outline-offset:-1px}
    .run-btn{align-self:flex-start;padding:6px 16px;background:var(--acc);color:#fff;border:0;border-radius:6px;font-weight:600;font-size:13px;cursor:pointer}
    .run-btn:hover{filter:brightness(1.1)}
    .response{margin-top:12px;background:var(--bg1);border:1px solid var(--line);border-radius:6px;padding:12px;font-family:var(--mono);font-size:12px;white-space:pre-wrap;max-height:320px;overflow-y:auto;color:var(--t0)}
    .tag-section{display:none}
    .tag-section.active{display:block}
    .pill{display:inline-block;padding:1px 8px;border-radius:12px;font-size:11px;font-weight:600;background:var(--bg2);color:var(--t1);margin:0 4px 4px 0;cursor:pointer;border:1px solid var(--line)}
    .pill.active{background:color-mix(in srgb,var(--acc) 22%,transparent);color:var(--acc);border-color:var(--acc)}
    .ws-note{background:color-mix(in srgb,var(--acc) 8%,transparent);border:1px solid color-mix(in srgb,var(--acc) 30%,transparent);border-radius:8px;padding:16px;margin-bottom:24px;font-size:13px;color:var(--t1)}
    .ws-note a{color:var(--acc)}
    #token-row{display:flex;gap:8px;align-items:center;margin-bottom:24px}
    #token-input{flex:1;padding:8px 10px;background:var(--bg1);border:1px solid var(--line);border-radius:6px;color:var(--t0);font-family:var(--mono);font-size:12px}
    #token-input:focus{outline:2px solid var(--acc);outline-offset:-1px}
    #load-status{font-size:12px;color:var(--t2)}
  </style>
</head>
<body>
<nav id="nav">
  <h2>Virta API</h2>
  <ul id="nav-list"></ul>
</nav>
<main>
  <h1>Virta local API</h1>
  <p class="subtitle" id="subtitle">Loading…</p>

  <div id="token-row">
    <input id="token-input" placeholder="Bearer token (paste from discovery file or Settings → Integrations)" spellcheck="false">
    <span id="load-status"></span>
  </div>

  <div class="ws-note">
    <b>WebSocket event stream</b> — connect to <code>/v1/stream?token=&lt;token&gt;</code>, then send<br>
    <code>{"action":"subscribe","channels":["twitch:forsen"],"since":0}</code><br>
    Full protocol: <a href="/v1/asyncapi.json">/v1/asyncapi.json</a>
  </div>

  <div id="tags-filter" style="margin-bottom:24px"></div>
  <div id="sections"></div>
</main>
<script>
const q = s => document.querySelector(s), all = s => document.querySelectorAll(s);
let spec = null, activeTag = 'all';

function scopeColor(scope) {
  return {read:'#3fb950',send:'#5b8cff',moderate:'#d29922',control:'#9aa1ae',admin:'#f85149'}[scope]||'#5c6370';
}

function token() { return q('#token-input').value.trim(); }

function renderNav(tags) {
  const ul = q('#nav-list');
  ul.innerHTML = '<li><a href="#" class="active" data-tag="all">All endpoints</a></li>';
  tags.forEach(t => {
    const li = document.createElement('li');
    li.innerHTML = '<a href="#" data-tag="'+t.name+'">'+t.name+'</a>';
    ul.appendChild(li);
  });
  ul.addEventListener('click', e => {
    const a = e.target.closest('a[data-tag]');
    if (!a) return;
    e.preventDefault();
    activeTag = a.dataset.tag;
    all('#nav-list a').forEach(x => x.classList.remove('active'));
    a.classList.add('active');
    all('.tag-section').forEach(s => s.classList.remove('active'));
    const target = activeTag === 'all' ? 'all' : activeTag;
    if (target === 'all') all('.tag-section').forEach(s => s.classList.add('active'));
    else { const sec = document.getElementById('sec-'+target); if(sec) sec.classList.add('active'); }
  });
}

function renderEndpoint(method, path, op) {
  const div = document.createElement('div');
  div.className = 'endpoint';
  const params = (op.parameters||[]).map(p => '{'+p.name+'}');
  const hasBody = ['POST','PUT'].includes(method);
  div.innerHTML = '<div class="endpoint-head">'
    +'<span class="method '+method+'">'+method+'</span>'
    +'<code class="path">'+path+'</code>'
    +'<span class="summary">'+op.summary+'</span>'
    +'<span class="scope-badge" style="color:'+scopeColor(op["x-required-scope"])+'">'+op["x-required-scope"]+'</span>'
    +'<button class="try-btn">Try</button>'
    +'</div>'
    +'<div class="endpoint-body">'
    +'<div class="try-area">'
    +(params.length ? '<label>Path params</label><input class="params-input" placeholder="e.g. id=abc123">' : '')
    +(hasBody ? '<label>Request body (JSON)</label><textarea class="body-input" rows="3" placeholder="{}"></textarea>' : '')
    +'<button class="run-btn">Send request</button>'
    +'<pre class="response" style="display:none"></pre>'
    +'</div></div>';
  div.querySelector('.endpoint-head').addEventListener('click', e => {
    if (e.target.closest('.try-btn')) return;
    const body = div.querySelector('.endpoint-body');
    body.classList.toggle('open');
  });
  div.querySelector('.try-btn').addEventListener('click', e => {
    e.stopPropagation();
    div.querySelector('.endpoint-body').classList.add('open');
  });
  div.querySelector('.run-btn').addEventListener('click', async () => {
    let url = path;
    const pi = div.querySelector('.params-input');
    if (pi) (pi.value||'').split('&').forEach(p => { const [k,v]=(p+'=').split('='); url=url.replace('{'+k.trim()+'}', encodeURIComponent(v.trim())); });
    const resp = div.querySelector('.response');
    resp.style.display = 'block';
    resp.textContent = 'Loading…';
    try {
      const opts = { method, headers: { Authorization: 'Bearer '+token() } };
      const bi = div.querySelector('.body-input');
      if (bi && bi.value.trim()) { opts.headers['Content-Type']='application/json'; opts.body=bi.value.trim(); }
      const r = await fetch(url, opts);
      const text = await r.text();
      let pretty = text;
      try { pretty = JSON.stringify(JSON.parse(text), null, 2); } catch{}
      resp.textContent = r.status+' '+r.statusText+'\n\n'+pretty;
    } catch(e) { resp.textContent = 'Error: '+e.message; }
  });
  return div;
}

async function load() {
  q('#load-status').textContent = 'Loading spec…';
  try {
    const r = await fetch('/v1/openapi.json');
    spec = await r.json();
    q('#subtitle').textContent = spec.info.description.split('.')[0]+'. Version '+spec.info.version+' · '+spec.info["x-build"];
    const tags = spec.tags||[];
    renderNav(tags);
    const sections = q('#sections');
    const byTag = {};
    tags.forEach(t => { byTag[t.name] = []; });
    Object.entries(spec.paths||{}).forEach(([path, item]) => {
      Object.entries(item).forEach(([method, op]) => {
        const tag = (op.tags||['other'])[0];
        if (!byTag[tag]) byTag[tag] = [];
        byTag[tag].push({method:method.toUpperCase(), path, op});
      });
    });
    tags.forEach(t => {
      const sec = document.createElement('div');
      sec.className = 'tag-section active';
      sec.id = 'sec-'+t.name;
      sec.innerHTML = '<div class="section"><h2>'+t.name+'<small style="font-size:13px;font-weight:400;color:var(--t2);margin-left:8px">'+t.description+'</small></h2></div>';
      const inner = sec.querySelector('.section');
      (byTag[t.name]||[]).forEach(({method,path,op}) => inner.appendChild(renderEndpoint(method,path,op)));
      sections.appendChild(sec);
    });
    q('#load-status').textContent = Object.values(byTag).flat().length+' endpoints';
  } catch(e) {
    q('#load-status').textContent = 'Failed to load spec: '+e.message;
  }
}
load();
</script>
</body>
</html>
`
