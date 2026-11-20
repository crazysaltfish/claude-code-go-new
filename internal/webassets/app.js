'use strict';
const $ = id => document.getElementById(id);
const state = {id:null,config:null,chats:[],busy:false,source:null,tools:new Map(),assistants:new Map(),approvals:new Map(),generation:0,transcript:[],lastEvent:0};
const text = (tag, value, className) => {const node=document.createElement(tag);node.textContent=value??'';if(className)node.className=className;return node;};
const pretty = value => typeof value==='string'?value:JSON.stringify(value,null,2);
function toast(message){$('toast').textContent=message;$('toast').classList.remove('hidden');clearTimeout(toast.timer);toast.timer=setTimeout(()=>$('toast').classList.add('hidden'),4500);}
async function api(path,body){
  const res=await fetch(path,body===undefined?{}:{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  const data=await res.json();if(!res.ok)throw Error(data.error||'请求失败');return data;
}
function setBusy(busy){state.busy=busy;$('send').classList.toggle('hidden',busy);$('stop').classList.toggle('hidden',!busy);$('send').disabled=busy||!$('prompt').value.trim()||!state.id;$('run-status').classList.toggle('busy',busy);$('run-status').textContent=busy?'正在处理 · 工具调用会在对话中展示':'';}
function nearBottom(){const area=$('scroll-area');return area.scrollHeight-area.scrollTop-area.clientHeight<140;}
function scrollBottom(){const area=$('scroll-area');area.scrollTop=area.scrollHeight;}
function inline(parent,value){
  const pattern=/(`[^`]+`|\*\*[^*]+\*\*|\[[^\]]+\]\(https?:\/\/[^\s)]+\))/g;
  let last=0;for(const match of value.matchAll(pattern)){parent.append(document.createTextNode(value.slice(last,match.index)));const chunk=match[0];if(chunk.startsWith('`'))parent.append(text('code',chunk.slice(1,-1)));else if(chunk.startsWith('**'))parent.append(text('strong',chunk.slice(2,-2)));else {const parts=chunk.match(/^\[([^\]]+)\]\((.*)\)$/);const link=text('a',parts[1]);link.href=parts[2];link.target='_blank';link.rel='noopener noreferrer';parent.append(link);}last=match.index+chunk.length;}parent.append(document.createTextNode(value.slice(last)));
}
function markdown(parent,value){
  parent.replaceChildren();const lines=value.split('\n');let paragraph=[];let list=null;
  function flush(){if(paragraph.length){const p=document.createElement('p');inline(p,paragraph.join('\n'));parent.append(p);paragraph=[];}list=null;}
  for(let i=0;i<lines.length;i++){
    let line=lines[i];
    if(line.startsWith('```')){flush();const language=line.slice(3).trim();const code=[];while(++i<lines.length&&!lines[i].startsWith('```'))code.push(lines[i]);const pre=document.createElement('pre');const node=text('code',code.join('\n'));if(language)node.dataset.language=language;pre.append(node);parent.append(pre);continue;}
    if(/^\s*\|/.test(line)&&i+1<lines.length&&/^\s*\|?[\s:|-]+\|/.test(lines[i+1])){flush();const table=document.createElement('table');const cells=s=>s.trim().replace(/^\||\|$/g,'').split('|');const head=document.createElement('tr');cells(line).forEach(v=>{const th=document.createElement('th');inline(th,v.trim());head.append(th);});table.append(head);i++;while(i+1<lines.length&&/^\s*\|/.test(lines[i+1])){const row=document.createElement('tr');cells(lines[++i]).forEach(v=>{const td=document.createElement('td');inline(td,v.trim());row.append(td);});table.append(row);}parent.append(table);continue;}
    const heading=line.match(/^(#{1,6})\s+(.*)/);if(heading){flush();const h=document.createElement(heading[1].length<=2?'h2':'h3');inline(h,heading[2]);parent.append(h);continue;}
    const item=line.match(/^\s*(?:[-*]|\d+\.)\s+(.*)/);if(item){if(paragraph.length)flush();if(!list){list=document.createElement('ul');parent.append(list);}const li=document.createElement('li');inline(li,item[1]);list.append(li);continue;}
    if(!line.trim()){flush();continue;}if(list)flush();paragraph.push(line);
  }flush();
}
function hideWelcome(){$('welcome')?.remove();}
function message(role,value,key){
  hideWelcome();const follow=nearBottom();const article=text('article','',`message ${role}`);let body=text('div','','message-body');
  if(role==='assistant'){const heading=text('div','','assistant-heading');heading.append(text('span','✳','star'),text('span',state.config?.model||'Claude Code'));article.append(heading);markdown(body,value);}else body.textContent=value;
  article.append(body);if(role==='assistant'){const actions=text('div','','message-actions');const copy=text('button','复制');copy.onclick=()=>navigator.clipboard.writeText(article.dataset.content||'').then(()=>toast('已复制')).catch(()=>toast('无法访问剪贴板'));actions.append(copy);article.append(actions);}
  article.dataset.content=value;$('conversation').append(article);state.transcript.push({role,article});if(key)state.assistants.set(key,{article,body,value});if(follow)scrollBottom();return {article,body,value};
}
function assistantDelta(id,delta){if(!delta)return;const follow=nearBottom();let entry=state.assistants.get(id);if(!entry)entry=message('assistant','',id);entry.value+=delta;entry.article.dataset.content=entry.value;if(!entry.frame)entry.frame=requestAnimationFrame(()=>{entry.frame=null;markdown(entry.body,entry.value);if(follow)scrollBottom();});}
function assistantFinal(id,blocks){
  const value=blocks.filter(b=>b.type==='text').map(b=>b.text).join('\n');let entry=state.assistants.get(id);
  if(value){if(!entry)entry=message('assistant',value,id);else {if(entry.frame)cancelAnimationFrame(entry.frame);entry.frame=null;entry.value=value;entry.article.dataset.content=value;markdown(entry.body,value);}}
  blocks.filter(b=>b.type==='tool_use').forEach(toolUse);
}
function toolUse(block){
  if(state.tools.has(block.id))return;hideWelcome();const card=text('details','','tool-card');const summary=document.createElement('summary');const input=block.input||{};const detail=input.description||input.file_path||input.target_file||input.command||input.pattern||input.query||'';
  summary.append(text('span',block.name+'  '+detail,'tool-label'),text('span','执行中','tool-status'));card.append(summary,text('pre','输入\n'+pretty(input)));const output=text('pre','等待执行结果…');card.append(output);$('conversation').append(card);state.tools.set(block.id,{card,output,summary});if(nearBottom())scrollBottom();
}
function toolResult(data){
  let entry=state.tools.get(data.tool_use_id);if(!entry){toolUse({id:data.tool_use_id,name:data.tool_name||'Tool',input:{}});entry=state.tools.get(data.tool_use_id);}
  entry.output.textContent='输出\n'+pretty(data.content)+(data.display?.diff?'\n\n'+data.display.diff:'')+(data.truncated?'\n\n[工具输出已截断]':'');entry.summary.querySelector('.tool-status').textContent=data.is_error?'失败':'已完成';entry.card.classList.toggle('failed',!!data.is_error);
}
function notice(value){hideWelcome();$('conversation').append(text('div',value,'notice'));}
function replayHistory(history){for(const item of history||[]){const blocks=Array.isArray(item.content)?item.content:[];const value=blocks.filter(b=>b.type==='text').map(b=>b.text).join('\n');if(value&&!value.startsWith('Automatic sub-agent update'))message(item.role==='assistant'?'assistant':'user',value);blocks.filter(b=>b.type==='tool_use').forEach(toolUse);blocks.filter(b=>b.type==='tool_result').forEach(b=>toolResult({tool_use_id:b.tool_use_id,content:b.content,is_error:b.is_error}));}}
function handleEvent(event){
  const {kind,data}=event;
  if(kind==='busy'){setBusy(data);if(!data)refreshChats().catch(()=>{});return;}
  if(kind==='notice'){notice(data);return;}
  if(kind==='error'){toast(data);notice('执行错误：'+data);return;}
  if(kind==='approval'){approval(data);return;}
  if(kind==='approved'){removeApproval(data);return;}
  if(kind==='result'){if(data.is_error){notice('执行结束：'+data.result);toast(data.result);}else if(data.duration_ms) $('run-status').textContent=`已完成 · ${(data.duration_ms/1000).toFixed(1)} 秒`;return;}
  if(kind!=='sdk')return;
  switch(data.type){
    case 'user': {const body=data.message;const value=(body?.content||[]).filter(b=>b.type==='text').map(b=>b.text).join('\n');if(value&&!value.startsWith('Automatic sub-agent update'))message('user',value);break;}
    case 'assistant_delta':assistantDelta(data.uuid,data.message?.text);break;
    case 'assistant':assistantFinal(data.uuid,data.message?.content||[]);break;
    case 'tool_result':toolResult(data.message);break;
    case 'tool_progress':{const t=state.tools.get(data.message?.tool_use_id);if(t)t.summary.querySelector('.tool-status').textContent='执行中';break;}
    case 'system':if(data.message?.error){toast(data.message.error);}break;
  }
}
function approval(data){
  if(state.approvals.has(data.id))return;
  const card=text('section','','approval');card.append(text('strong','需要批准 · '+data.tool));if(data.risk)card.append(text('p',data.risk));card.append(text('pre',pretty(data.input)));const actions=text('div','','approval-actions');
  for(const [label,decision] of [['允许本次','allow_once'],['允许本会话','allow_session'],['拒绝','deny']]){const button=text('button',label);button.onclick=async()=>{try{await api(`/api/chats/${state.id}/approve`,{id:data.id,decision});removeApproval(data.id);}catch(error){toast(error.message);}};actions.append(button);}
  card.append(actions);$('approvals').append(card);state.approvals.set(data.id,card);
}
function removeApproval(id){state.approvals.get(id)?.remove();state.approvals.delete(id);}
const agentLabels = {running:'运行中', pending:'等待中', completed:'已完成', failed:'失败', killed:'已停止', interrupted:'已中断'};
function agentPhase(task) {
  if (task.status === 'failed') return task.error || '执行失败';
  if (['completed', 'killed', 'interrupted'].includes(task.status)) return agentLabels[task.status];
  return task.progress?.summary || '等待执行';
}

let taskListSnapshot = '';
function renderTasks(tasks) {
  const all = Object.values(tasks || {}).filter(task => task.type === 'local_agent');
  const priority = task => ['running', 'pending'].includes(task.status) ? 0 : 1;
  all.sort((a, b) => priority(a) - priority(b) || new Date(b.startTime) - new Date(a.startTime));
  const snapshot = JSON.stringify(all.map(task => [
    task.id, task.status, task.description, agentPhase(task), task.progress?.lastActivity,
  ]));
  if (snapshot === taskListSnapshot) return;
  taskListSnapshot = snapshot;
  const list = $('task-list');
  const focusedID = list.contains(document.activeElement) ? document.activeElement.dataset.task : null;
  $('task-count').textContent = all.length;
  $('running-count').textContent = all.filter(task => task.status === 'running').length + ' running';
  list.replaceChildren();
  if (!all.length) {
    list.append(text('p', '还没有子 Agent 任务。你可以在对话中委派独立工作。', 'empty-tasks'));
    return;
  }
  for (const task of all) {
    const card = text('button', '', `task task-card ${task.status}`);
    card.type = 'button';
    card.dataset.task = task.id;
    card.setAttribute('aria-haspopup', 'dialog');
    const top = text('span', '', 'task-card-top');
    top.append(text('span', agentLabels[task.status] || task.status, 'task-state'),
      text('span', '查看详情', 'task-card-link'));
    card.append(top, text('span', task.description, 'task-card-title'),
      text('span', agentPhase(task), 'task-card-summary'));
    const last = task.progress?.lastActivity;
    if (last) {
      card.append(text('span', last.toolName + (last.activityDescription ? ': ' + last.activityDescription : ''), 'task-card-summary'));
    }
    card.onclick = () => openAgentDetail(task);
    list.append(card);
    if (task.id === focusedID) card.focus({preventScroll:true});
  }
}

let selectedAgent = null;
const agentDialog = $('agent-detail');
const timeLabel = value => value ? new Date(value).toLocaleTimeString() : '';

function openAgentDetail(task) {
  selectedAgent = {chat: state.id, id: task.id, snapshot: '', loading: false};
  $('agent-detail-title').textContent = task.description || '任务详情';
  const body = $('agent-detail-body');
  body.replaceChildren(text('p', '正在加载执行详情…'));
  body.setAttribute('aria-busy', 'true');
  if (!agentDialog.open) agentDialog.showModal();
  refreshAgentDetail();
}

async function refreshAgentDetail() {
  const selection = selectedAgent;
  if (!selection || selection.loading || !agentDialog.open) return;
  selection.loading = true;
  try {
    const data = await api(`/api/chats/${encodeURIComponent(selection.chat)}/agent?task=${encodeURIComponent(selection.id)}`);
    if (selection !== selectedAgent || !agentDialog.open) return;
    const snapshot = JSON.stringify(data);
    if (selection.snapshot !== snapshot) {
      renderAgentDetail(data);
      selection.snapshot = snapshot;
    }
  } catch (error) {
    if (selection === selectedAgent && !selection.snapshot) {
      $('agent-detail-body').replaceChildren(text('p', '无法加载执行详情：' + error.message));
    }
  } finally {
    selection.loading = false;
    if (selection === selectedAgent) $('agent-detail-body').setAttribute('aria-busy', 'false');
  }
}

function renderAgentDetail({task, entries, result}) {
  const body = $('agent-detail-body');
  const scrollTop = body.scrollTop;
  const expanded = new Set(Array.from(body.querySelectorAll('details[open]'), detail => detail.dataset.entry));
  const focusedEntry = body.contains(document.activeElement) ? document.activeElement.closest('details')?.dataset.entry : null;
  const meta = text('div', '', 'agent-detail-meta');
  meta.append(text('span', agentLabels[task.status] || task.status),
    text('span', task.agentType || '通用 Agent'),
    text('span', task.isBackgrounded ? '后台执行' : '前台执行'),
    text('span', '任务 ID ' + task.id));
  if (task.model) meta.append(text('span', task.model));
  if (task.startTime) meta.append(text('span', '开始 ' + timeLabel(task.startTime)));
  if (task.endTime) meta.append(text('span', '结束 ' + timeLabel(task.endTime)));

  const responsibility = text('section', '', 'agent-section');
  responsibility.append(text('h3', '职责与委派指令'), text('pre', task.prompt || task.description || '暂无委派指令'));

  const process = text('section', '', 'agent-section');
  process.append(text('h3', '执行过程'));
  const timeline = text('ol', '', 'agent-timeline');
  let focusTarget;
  for (const entry of entries || []) {
    const item = text('li', '', 'agent-event' + (entry.output?.failed ? ' failed' : ''));
    const timestamp = text('span', timeLabel(entry.time), 'agent-event-time');
    if (entry.kind === 'message') {
      const heading = text('div', 'Agent 回复', 'agent-event-heading');
      heading.append(timestamp);
      item.append(heading, text('p', entry.text + (entry.truncated ? '\n[内容已截断]' : ''), 'agent-message'));
    } else {
      const detail = document.createElement('details');
      detail.dataset.entry = String(entry.id);
      detail.open = expanded.has(detail.dataset.entry);
      const result = entry.output;
      const status = result ? (result.failed ? '失败' : '已返回') :
        ['running', 'pending'].includes(task.status) ? '等待输出' : '无返回记录';
      const summary = text('summary', `${entry.tool || '工具'} · ${status}`);
      summary.append(timestamp);
      detail.append(summary);
      if (entry.kind === 'tool_call') {
        let input = entry.text;
        if (!entry.truncated) {
          try { input = pretty(JSON.parse(input)); } catch { /* Preserve the original arguments. */ }
        }
        detail.append(text('div', '输入', 'agent-tool-label'),
          text('pre', input + (entry.truncated ? '\n[内容已截断]' : '')));
      }
      const outputLabel = text('div', '输出', 'agent-tool-label');
      if (result?.time) outputLabel.append(text('span', timeLabel(result.time), 'agent-event-time'));
      detail.append(outputLabel);
      if (result) {
        detail.append(text('pre', (result.text || '（空输出）') + (result.truncated ? '\n[内容已截断]' : '')));
      } else {
        detail.append(text('p', status, 'agent-detail-phase'));
      }
      if (detail.dataset.entry === focusedEntry) focusTarget = summary;
      item.append(detail);
    }
    timeline.append(item);
  }
  if (timeline.children.length) process.append(timeline);
  else process.append(text('p', '尚无执行记录。', 'agent-detail-phase'));

  body.replaceChildren(meta, text('p', agentPhase(task), 'agent-detail-phase'), responsibility, process);
  if (result || task.error) {
    const outcome = text('section', '', 'agent-section');
    outcome.append(text('h3', task.error ? '执行错误' : '最终结果'), text('pre', task.error || result));
    body.append(outcome);
  }
  body.scrollTop = scrollTop;
  focusTarget?.focus({preventScroll:true});
}

$('close-agent-detail').onclick = () => agentDialog.close();
agentDialog.addEventListener('click', event => {
  if (event.target !== agentDialog) return;
  const bounds = agentDialog.getBoundingClientRect();
  if (event.clientX < bounds.left || event.clientX > bounds.right ||
      event.clientY < bounds.top || event.clientY > bounds.bottom) agentDialog.close();
});
agentDialog.addEventListener('close', () => {
  if (agentDialog.open) return;
  const id = selectedAgent?.id;
  selectedAgent = null;
  for (const card of $('task-list').querySelectorAll('button')) {
    if (card.dataset.task === id) card.focus({preventScroll:true});
  }
});
setInterval(refreshAgentDetail, 1200);

function chatTitle(chat){return !chat?.title||chat.title==='Untitled session'?'新对话':chat.title;}
function renderChats(){const filter=$('search').value.trim().toLowerCase();$('chat-list').replaceChildren();for(const chat of state.chats){if(!chatTitle(chat).toLowerCase().includes(filter))continue;const button=text('button',chatTitle(chat),'chat-link'+(chat.id===state.id?' selected':''));button.title=chatTitle(chat);button.onclick=()=>selectChat(chat.id).catch(error=>toast(error.message));$('chat-list').append(button);}$('chat-total').textContent=state.chats.length;}
async function refreshChats(){state.chats=(await api('/api/chats'))||[];renderChats();const active=state.chats.find(c=>c.id===state.id);$('chat-title').textContent=chatTitle(active);}
async function selectChat(id){
  selectedAgent = null;
  if (agentDialog.open) agentDialog.close();
  renderTasks({});
  const generation=++state.generation;state.source?.close();state.id=id;state.lastEvent=0;state.tools.clear();state.assistants.clear();state.approvals.clear();state.transcript=[];$('conversation').replaceChildren();$('approvals').replaceChildren();setBusy(false);renderChats();
  document.dispatchEvent(new Event('web-chat-changed'));
  const data=await api(`/api/chats/${id}/state`);if(generation!==state.generation)return;replayHistory(data.history);renderTasks(data.tasks);(data.approvals||[]).forEach(approval);setBusy(data.busy);
  if(!data.history?.length){const welcome=welcomeTemplate.cloneNode(true);$('conversation').append(welcome);bindSuggestions();}
  const source=new EventSource(`/api/chats/${id}/events`);state.source=source;source.onopen=()=>$('connection-dot').classList.remove('offline');
  source.onerror=()=>{$('connection-dot').classList.add('offline');$('run-status').textContent='连接中断，正在重连…';};
  source.onmessage=event=>{if(generation!==state.generation)return;try{const data=JSON.parse(event.data);if(data.id<=state.lastEvent)return;state.lastEvent=data.id;handleEvent(data);}catch(error){toast('无法读取服务事件：'+error.message);}};
  history.replaceState(null,'','#'+encodeURIComponent(id));if(innerWidth<=760)$('sidebar').classList.add('collapsed');if(data.history?.length)scrollBottom();else $('scroll-area').scrollTop=0;
}
const welcomeTemplate=$('welcome').cloneNode(true);
function bindSuggestions(){document.querySelectorAll('[data-prompt]').forEach(button=>button.onclick=()=>{$('prompt').value=button.dataset.prompt;$('prompt').dispatchEvent(new Event('input'));$('prompt').focus();});}
bindSuggestions();
$('prompt').addEventListener('input',()=>{$('send').disabled=state.busy||!state.id||!$('prompt').value.trim();$('prompt').style.height='auto';$('prompt').style.height=Math.min($('prompt').scrollHeight,180)+'px';});
$('prompt').addEventListener('keydown',event=>{if(event.key==='Enter'&&!event.shiftKey&&!event.isComposing){event.preventDefault();$('composer').requestSubmit();}});
$('composer').onsubmit=async event=>{event.preventDefault();const value=$('prompt').value.trim();if(!value||state.busy||!state.id)return;const id=state.id;setBusy(true);try{await api(`/api/chats/${id}/messages`,{message:value});if(state.id===id){$('prompt').value='';$('prompt').style.height='auto';}}catch(error){if(state.id===id)setBusy(false);toast(error.message);}};
$('stop').onclick=async()=>{try{await api(`/api/chats/${state.id}/stop`,{});toast('已请求停止执行');}catch(error){toast(error.message);}};
$('new-chat').onclick=async()=>{try{const data=await api('/api/chats',{});await refreshChats();await selectChat(data.id);$('prompt').focus();}catch(error){toast(error.message);}};
$('search').oninput=renderChats;
$('toggle-sidebar').onclick=()=>$('sidebar').classList.toggle('collapsed');
$('close-sidebar').onclick=()=>$('sidebar').classList.add('collapsed');
function showSidePanel(name){
  document.dispatchEvent(new CustomEvent('web-side-panel', {detail:name}));
}
for (const id of ['tasks-button','toggle-tasks']) $(id).onclick=()=>{
  if (!$('task-panel').classList.toggle('hidden')) showSidePanel('tasks');
};
$('close-tasks').onclick=()=>$('task-panel').classList.add('hidden');
document.addEventListener('web-side-panel', event => {
  if (event.detail !== 'tasks') $('task-panel').classList.add('hidden');
});
$('export-chat').onclick=()=>{const content=state.transcript.map(m=>(m.role==='user'?'## 你':'## Claude Code')+'\n\n'+m.article.dataset.content).join('\n\n');const url=URL.createObjectURL(new Blob([content],{type:'text/markdown;charset=utf-8'}));const a=document.createElement('a');a.href=url;a.download='conversation.md';a.click();setTimeout(()=>URL.revokeObjectURL(url),5000);};
document.addEventListener('keydown',event=>{if((event.metaKey||event.ctrlKey)&&event.key==='k'){event.preventDefault();$('new-chat').click();}});
if(innerWidth<=760)$('sidebar').classList.add('collapsed');
async function initialize(){try{state.config=await api('/api/config');$('model-label').textContent=state.config.model;$('project-name').textContent=state.config.project.split('/').filter(Boolean).pop();$('project-chip').textContent='▱ '+$('project-name').textContent;await refreshChats();const requested=decodeURIComponent(location.hash.slice(1));await selectChat(state.chats.some(c=>c.id===requested)?requested:state.config.initial_chat);}catch(error){toast(error.message);$('run-status').textContent='无法连接本地服务，请刷新重试';}}
initialize();
setInterval(async()=>{const id=state.id,generation=state.generation;if(!id)return;try{const data=await api(`/api/chats/${id}/state`);if(generation!==state.generation)return;renderTasks(data.tasks);const pending=new Set((data.approvals||[]).map(a=>a.id));for(const key of state.approvals.keys())if(!pending.has(key))removeApproval(key);(data.approvals||[]).forEach(approval);}catch{}},1200);
