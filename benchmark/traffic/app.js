const $ = id => document.getElementById(id);
let tools = [];
async function api(path, body) {
  const response = await fetch('/api/' + path, body === undefined ? {} : {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)});
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Request failed');
  return data;
}
function message(error) { $('message').textContent = error.message || error; }
function options(element, items, placeholder) {
  element.replaceChildren(new Option(placeholder, ''));
  for (const item of items) element.add(new Option(item.name, item.slug || item.name));
}
async function servers() { try { options($('server'), await api('servers'), 'Select a virtual server'); } catch(e) { message(e); } }
function clearTools() { tools=[]; options($('tool'), [], 'Discover tools first'); $('schema').textContent=''; $('description').textContent=''; }
$('server').onchange = clearTools;
$('key').oninput = clearTools;
$('refresh').onclick = () => {clearTools(); servers();};
$('discover').onclick = async () => {
  $('discover').disabled=true;
  try { tools=await api('tools', {server:$('server').value,key:$('key').value}); options($('tool'), tools, 'Select a tool'); message(`${tools.length} tools available`); } catch(e) {message(e);} finally {$('discover').disabled=false;}
};
$('tool').onchange = () => { const tool=tools.find(t=>t.name===$('tool').value); $('description').textContent=tool?.description || ''; $('schema').textContent=JSON.stringify(tool?.inputSchema || {}, null, 2); };
$('form').onsubmit = async event => {
  event.preventDefault(); $('start').disabled=true;
  try {
    const argumentsValue=JSON.parse($('arguments').value);
    await api('start', {server:$('server').value,tool:$('tool').value,key:$('key').value,arguments:argumentsValue,rate:Number($('rate').value),duration:Number($('duration').value),workers:Number($('workers').value)});
    $('key').value=''; message('Traffic started. Agent key cleared from the form.');
  } catch(e) {message(e); $('start').disabled=false;}
};
$('stop').onclick = async () => {try {await api('stop', {}); message('Stopping new requests; waiting for in-flight work.');} catch(e) {message(e);}};
async function poll() {
  try {const status=await api('status'); $('stats').textContent=JSON.stringify(status,null,2); $('start').disabled=['starting','running','draining'].includes(status.state);} catch(e) {message(e);}
  setTimeout(poll,1000);
}
servers(); poll();
