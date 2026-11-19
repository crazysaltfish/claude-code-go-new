'use strict';

const byID = id => document.getElementById(id);
const map = byID('map');
const scene = byID('scene');
const input = byID('question');
const form = byID('ask');
const detail = byID('detail-content');
const historyDialog = byID('history');
const svgNS = 'http://www.w3.org/2000/svg';
let canvas = null;
let selected = null;
let anchor = 0;
let request = null;
let navigation = 0;
let positions = new Map();
let camera = {x: 0, y: 0, scale: 1, auto: true};
let drag = null;

function dom(tag, value, className) {
  const element = document.createElement(tag);
  if (value != null) element.textContent = value;
  if (className) element.className = className;
  return element;
}

function svg(tag, attrs = {}, value) {
  const element = document.createElementNS(svgNS, tag);
  for (const [name, content] of Object.entries(attrs)) element.setAttribute(name, String(content));
  if (value != null) element.textContent = value;
  return element;
}

function addAction(element, label, action) {
  element.setAttribute('tabindex', '0');
  element.setAttribute('role', 'button');
  element.setAttribute('aria-label', label);
  element.addEventListener('click', () => action(false));
  element.addEventListener('keydown', event => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      action(true);
    }
  });
  element.append(svg('title', {}, label));
}

function svgLabel(parent, text, x, y, width, className) {
  const label = svg('text', {x, y, class: className});
  const lines = [''];
  let length = 0;
  let clipped = false;
  for (const char of text.replace(/\s+/gu, ' ')) {
    const size = char.codePointAt(0) > 255 ? 2 : 1;
    if (length + size > width) {
      if (lines.length === 2) {
        clipped = true;
        break;
      }
      lines.push('');
      length = 0;
    }
    lines[lines.length - 1] += char;
    length += size;
  }
  lines.forEach((line, index) => {
    if (clipped && index === lines.length - 1) line = [...line].slice(0, -1).join('') + '…';
    label.append(svg('tspan', {x, dy: index === 0 ? 0 : 15}, line));
  });
  parent.append(label);
}

function status(message, error = false) {
  const line = byID('status');
  line.textContent = message;
  line.classList.toggle('error', error);
}

async function api(path, body, signal) {
  const options = {signal};
  if (body !== undefined) {
    options.method = 'POST';
    options.headers = {'Content-Type': 'application/json'};
    options.body = JSON.stringify(body);
  }
  const response = await fetch(path, options);
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw Error(payload.error || `请求失败（${response.status}）`);
  }
  return response;
}

function lineage(node) {
  const byNode = new Map(canvas.nodes.map(item => [item.id, item]));
  const path = [];
  for (let current = node; current; current = byNode.get(current.parent_id)) path.unshift(current);
  return path;
}

const mapWidth = 196;
const mapHeight = 64;

function organize() {
  const children = new Map();
  for (const node of canvas.nodes) {
    if (!children.has(node.parent_id)) children.set(node.parent_id, []);
    children.get(node.parent_id).push(node);
  }
  let row = 0;
  const layout = new Map();
  function place(node, depth) {
    const descendants = (children.get(node.id) || []).map(child => place(child, depth + 1));
    const y = descendants.length ? (descendants[0] + descendants.at(-1)) / 2 : row++ * 92;
    layout.set(node.id, {x: depth * 258, y});
    return y;
  }
  place(canvas.nodes[0], 0);
  return layout;
}

function renderMap() {
  scene.replaceChildren();
  positions = organize();
  const path = new Set(lineage(selected).map(node => node.id));
  const byNode = new Map(canvas.nodes.map(node => [node.id, node]));
  for (const node of canvas.nodes) {
    if (!node.parent_id) continue;
    const from = positions.get(node.parent_id);
    const to = positions.get(node.id);
    const x = from.x + mapWidth, a = from.y + mapHeight / 2, b = to.y + mapHeight / 2;
    scene.append(svg('path', {
      d: `M${x} ${a} C${x + 28} ${a},${to.x - 28} ${b},${to.x} ${b}`,
      class: path.has(node.id) ? 'map-edge trail' : 'map-edge',
    }));
  }
  canvas.nodes.forEach((node, index) => {
    const {x, y} = positions.get(node.id);
    const group = svg('g', {
      transform: `translate(${x},${y})`,
      class: `map-node${node.parent_id ? '' : ' root'}${selected.id === node.id ? ' selected' : ''}`,
      'data-map-node': node.id,
      'aria-pressed': String(selected.id === node.id),
    });
    const parent = byNode.get(node.parent_id);
    const point = parent?.concepts[node.anchor - 1]?.label;
    const caption = point && [...point].length > 13 ? [...point].slice(0, 12).join('') + '…' : point;
    group.append(svg('rect', {rx: 12, width: mapWidth, height: mapHeight}));
    group.append(svg('text', {x: 12, y: 17, class: 'node-kind'}, caption || (node.parent_id ? '分支追问' : '根问题')));
    group.append(svg('text', {x: mapWidth - 12, y: 17, 'text-anchor': 'end', class: 'node-index'}, String(index + 1)));
    svgLabel(group, node.title, 12, 36, 24, 'node-label');
    addAction(group, node.title, keyboard => {
      selectNode(node);
      if (keyboard) scene.querySelector(`[data-map-node="${node.id}"]`)?.focus();
    });
    scene.append(group);
  });
  applyCamera();
}

function applyCamera() {
  scene.setAttribute('transform', `translate(${camera.x},${camera.y}) scale(${camera.scale})`);
}

function fit() {
  if (!positions.size) return;
  const bounds = map.getBoundingClientRect();
  if (bounds.width < 48 || bounds.height < 78) return;
  const spots = [...positions.values()];
  const width = Math.max(...spots.map(spot => spot.x)) + mapWidth;
  const height = Math.max(...spots.map(spot => spot.y)) + mapHeight;
  const gutterTop = 52;
  const scale = Math.min(1.12, (bounds.width - 34) / width, (bounds.height - gutterTop - 17) / height);
  camera = {x: (bounds.width - width * scale) / 2, y: gutterTop + (bounds.height - gutterTop - height * scale) / 2, scale, auto: true};
  applyCamera();
}

function centerNode() {
  if (camera.auto || !positions.has(selected.id)) {
    fit();
    return;
  }
  const {x, y} = positions.get(selected.id);
  const bounds = map.getBoundingClientRect();
  camera.x = bounds.width / 2 - (x + mapWidth / 2) * camera.scale;
  camera.y = (bounds.height + 45) / 2 - (y + mapHeight / 2) * camera.scale;
  applyCamera();
}

function zoom(factor, x, y) {
  const rect = map.getBoundingClientRect();
  x ??= rect.width / 2;
  y ??= (rect.height + 45) / 2;
  const scale = Math.max(Math.min(0.025, camera.scale), Math.min(2.8, camera.scale * factor));
  const ratio = scale / camera.scale;
  camera = {x: x - (x - camera.x) * ratio, y: y - (y - camera.y) * ratio, scale, auto: false};
  applyCamera();
}

map.addEventListener('pointerdown', event => {
  if (event.button !== 0 || event.target.closest('[data-map-node]')) return;
  drag = {pointer: event.pointerId, x: event.clientX - camera.x, y: event.clientY - camera.y};
  map.setPointerCapture(event.pointerId);
  map.classList.add('dragging');
});
map.addEventListener('pointermove', event => {
  if (drag?.pointer !== event.pointerId) return;
  camera = {...camera, x: event.clientX - drag.x, y: event.clientY - drag.y, auto: false};
  applyCamera();
});
map.addEventListener('lostpointercapture', () => {
  drag = null;
  map.classList.remove('dragging');
});
map.addEventListener('wheel', event => {
  if (!canvas) return;
  event.preventDefault();
  const rect = map.getBoundingClientRect();
  zoom(Math.exp(-Math.sign(event.deltaY) * 0.16), event.clientX - rect.left, event.clientY - rect.top);
}, {passive: false});
byID('fit').onclick = fit;
byID('zoom-in').onclick = () => zoom(1.25);
byID('zoom-out').onclick = () => zoom(0.8);
new ResizeObserver(() => { if (camera.auto) fit(); }).observe(map);

function selectNode(node) {
  if (selected?.id !== node.id) anchor = 0;
  selected = node;
  renderMap();
  renderDetail();
  renderComposeContext();
}

function diagramPositions(node) {
  return node.concepts.map((_, index) => {
    const row = Math.floor(index / 2);
    const side = node.kind === 'flow' && row % 2 ? 1 - (index % 2) : index % 2;
    return {x: 24 + side * 248, y: 19 + row * 87};
  });
}

function diagramConnection(from, to) {
  const width = 142, height = 48;
  if (from.y === to.y) {
    const x1 = from.x < to.x ? from.x + width : from.x;
    const x2 = from.x < to.x ? to.x : to.x + width;
    const y = from.y + height / 2;
    return {path: `M${x1} ${y}H${x2}`, x: (x1 + x2) / 2, y: y - 6};
  }
  const x1 = from.x + width / 2, x2 = to.x + width / 2;
  const y1 = from.y < to.y ? from.y + height : from.y;
  const y2 = from.y < to.y ? to.y : to.y + height;
  const mid = (y1 + y2) / 2;
  if (from.x === to.x) {
    return {path: `M${x1} ${y1}V${y2}`, x: x1 + 40, y: mid + 3};
  }
  // Cross-column edges use the central gutter so they leave the boxes visible.
  const direction = Math.sign(to.x - from.x);
  const start = direction > 0 ? from.x + width : from.x;
  const end = direction > 0 ? to.x : to.x + width;
  const middle = (start + end) / 2;
  const a = from.y + height / 2, b = to.y + height / 2;
  return {path: `M${start} ${a}C${middle} ${a},${middle} ${b},${end} ${b}`, x: middle, y: (a + b) / 2 - 6};
}

function makeDiagram(node) {
  const figure = dom('div', null, 'figure');
  const height = 28 + Math.ceil(node.concepts.length / 2) * 87 - 17;
  const drawing = svg('svg', {viewBox: `0 0 438 ${height}`, 'aria-label': '交互 SVG 图解：' + node.title});
  const defs = svg('defs');
  const arrow = svg('marker', {id: 'diagram-arrow', orient: 'auto', markerWidth: 7, markerHeight: 7, refX: 6, refY: 3.5});
  arrow.append(svg('path', {d: 'M0 0L7 3.5L0 7z', fill: '#7d9a81'}));
  defs.append(arrow);
  drawing.append(defs);
  const points = diagramPositions(node);
  for (const edge of node.edges) {
    const connection = diagramConnection(points[edge.from - 1], points[edge.to - 1]);
    drawing.append(svg('path', {d: connection.path, class: 'edge', 'marker-end': 'url(#diagram-arrow)'}));
    drawing.append(svg('text', {x: connection.x, y: connection.y, 'text-anchor': 'middle', class: 'edge-label'}, edge.label));
  }
  node.concepts.forEach((concept, index) => {
    const position = points[index];
    const group = svg('g', {
      transform: `translate(${position.x},${position.y})`,
      class: 'concept',
      'data-concept': index + 1,
    });
    group.append(svg('rect', {width: 142, height: 48, rx: 10}));
    group.append(svg('text', {x: 9, y: 17, class: 'concept-number'}, String(index + 1)));
    svgLabel(group, concept.label, 25, 20, 17, 'concept-label');
    addAction(group, concept.label, () => chooseConcept(index + 1));
    drawing.append(group);
  });
  figure.append(drawing, dom('div', null, 'concept-detail'));
  return figure;
}

function renderDetail() {
  detail.replaceChildren();
  const path = lineage(selected);
  if (path.length > 1) {
    const breadcrumbs = dom('nav', null, 'breadcrumbs');
    breadcrumbs.setAttribute('aria-label', '追问路径');
    for (const parent of path.slice(0, -1)) {
      const link = dom('button', parent.title);
      link.type = 'button';
      link.onclick = () => selectNode(parent);
      breadcrumbs.append(link, dom('span', '›'));
    }
    if (selected.anchor) breadcrumbs.append(dom('span', path.at(-2).concepts[selected.anchor - 1].label));
    detail.append(breadcrumbs);
  }
  detail.append(dom('h2', selected.title), dom('p', selected.summary, 'summary'), makeDiagram(selected), makeSources(selected));
  byID('detail').scrollTop = 0;
  chooseConcept(anchor);
}

function chooseConcept(index) {
  anchor = index;
  const container = detail.querySelector('.concept-detail');
  container.replaceChildren();
  for (const element of detail.querySelectorAll('[data-concept]')) {
    const chosen = Number(element.getAttribute('data-concept')) === index;
    element.classList.toggle('chosen', chosen);
    element.setAttribute('aria-pressed', String(chosen));
  }
  if (index === 0) {
    container.append(dom('p', '点击图中的知识点，查看解释或沿它继续追问。', 'concept-hint'));
  } else {
    const concept = selected.concepts[index - 1];
    container.append(dom('h3', concept.label), dom('p', concept.detail));
    if (concept.refs?.length) container.append(dom('p', '资料 ' + concept.refs.map(ref => `[${ref}]`).join(' ')));
    const suggest = dom('button', '↳ ' + concept.question, 'button suggested');
    suggest.type = 'button';
    suggest.onclick = () => {
      if (request) return;
      input.value = concept.question;
      input.focus();
    };
    container.append(suggest);
  }
  renderComposeContext();
}

function makeSources(node) {
  const section = dom('details', null, 'sources');
  section.append(dom('summary', node.sources.length ? `参考资料 · ${node.sources.length}` : '暂无可核对来源'));
  const coverage = {
    brave: '来源来自网页搜索的摘录。可打开原文核对结论和适用范围。',
    wikipedia: '检索范围为中文和英文维基百科，覆盖范围有限。',
    example: '这份示例使用下列公开资料。',
    none: '未取得可用的检索资料。这份图解基于模型的一般知识，结论有待核对。',
  };
  section.append(dom('p', coverage[node.method] || '打开原文可核对结论。', node.sources.length ? 'coverage' : 'coverage unverified'));
  if (!node.sources.length) {
    section.open = true;
    return section;
  }
  const list = dom('ol');
  for (const source of node.sources) {
    const item = dom('li');
    const link = dom('a', source.title);
    link.href = source.url;
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    item.append(link, dom('p', source.excerpt));
    list.append(item);
  }
  section.append(list);
  return section;
}

function renderComposeContext() {
  const context = byID('compose-context');
  context.hidden = !selected;
  context.replaceChildren();
  const action = byID('submit');
  action.firstChild.textContent = selected ? '添加追问 ' : '生成图解 ';
  if (!selected) {
    input.placeholder = '你想弄明白什么？';
    return;
  }
  const target = selected.concepts[anchor - 1]?.label || selected.title;
  context.append(dom('span', `追问：${target}`));
  if (anchor) {
    const clear = dom('button', '改为追问整个节点', 'clear-focus');
    clear.type = 'button';
    clear.onclick = () => chooseConcept(0);
    context.append(clear);
  }
  input.placeholder = '你想沿这个知识点问什么？';
}

function showCanvas(next, node = next.nodes[0], retain = false) {
  canvas = next;
  if (!retain) camera.auto = true;
  byID('welcome').hidden = true;
  byID('book').hidden = false;
  byID('canvas-title').textContent = canvas.title;
  byID('node-count').textContent = `${canvas.nodes.length} 个问题`;
  selectNode(node);
  centerNode();
}

function showEmpty(focus) {
  ++navigation;
  canvas = selected = null;
  anchor = 0;
  positions.clear();
  scene.replaceChildren();
  detail.replaceChildren();
  byID('book').hidden = true;
  byID('welcome').hidden = false;
  input.value = '';
  status('');
  renderComposeContext();
  if (focus) input.focus();
}

const stageLabels = {
  planning: 'Agent 正在整理检索词…',
  searching: '正在检索并收集资料…',
  drawing: '正在综合资料并生成图解…',
};

function setBusy(busy, canCancel = false) {
  for (const id of ['submit', 'new', 'library', 'example']) byID(id).disabled = busy;
  input.readOnly = busy;
  byID('cancel').hidden = !(busy && canCancel);
}

async function consume(response) {
  const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
  let rest = '';
  let result = null;
  try {
    for (;;) {
      const {done, value} = await reader.read();
      if (done) break;
      const lines = (rest + value).split('\n');
      rest = lines.pop();
      for (const line of lines) {
        if (!line) continue;
        const update = JSON.parse(line);
        if (update.stage) status(stageLabels[update.stage] || '');
        if (update.error) throw Error(update.error);
        if (update.canvas) result = update.canvas;
      }
    }
  } finally {
    reader.releaseLock();
  }
  if (!result) throw Error('Agent 的响应提前结束，请重试。');
  return result;
}

form.addEventListener('submit', async event => {
  event.preventDefault();
  const question = input.value.trim();
  if (!question || request) return;
  ++navigation;
  request = new AbortController();
  const previousCanvas = canvas?.id;
  const body = {
    question,
    canvas_id: canvas?.id || '',
    parent_id: selected?.id || '',
    anchor,
  };
  setBusy(true, true);
  status(stageLabels.planning);
  try {
    const response = await api('/api/learn', body, request.signal);
    const next = await consume(response);
    showCanvas(next, next.nodes.at(-1), previousCanvas === next.id);
    input.value = '';
    status('已保存到学习画布。');
  } catch (error) {
    if (request.signal.aborted) status('已停止。问题可以重新提交。');
    else status(error.message, true);
  } finally {
    request = null;
    setBusy(false);
  }
});

input.addEventListener('keydown', event => {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    form.requestSubmit();
  }
});
byID('cancel').onclick = () => request?.abort();
byID('new').onclick = () => showEmpty(true);
byID('example').onclick = async () => {
  ++navigation;
  setBusy(true);
  try {
    const result = await api('/api/example', {});
    showCanvas(await result.json());
    status('示例已保存，可选择知识点继续追问。');
  } catch (error) {
    status(error.message, true);
  } finally {
    setBusy(false);
  }
};

async function openSaved(id, generation) {
  const response = await api(`/api/canvases/${encodeURIComponent(id)}`);
  const data = await response.json();
  if (generation !== navigation) return;
  showCanvas(data);
  input.value = '';
  status('');
  if (historyDialog.open) historyDialog.close();
}

byID('library').onclick = async () => {
  const list = byID('history-list');
  const line = byID('history-status');
  list.replaceChildren();
  line.textContent = '正在读取…';
  historyDialog.showModal();
  try {
    const response = await api('/api/canvases');
    const data = await response.json();
    line.textContent = data.length ? '' : '还没有已保存的学习画布。';
    for (const item of data) {
      const row = dom('li');
      const button = dom('button', item.title, 'button');
      button.type = 'button';
      button.append(dom('small', `${item.count} 个问题`));
      button.onclick = () => {
        const generation = ++navigation;
        openSaved(item.id, generation).catch(error => {
          if (generation === navigation) line.textContent = error.message;
        });
      };
      row.append(button);
      list.append(row);
    }
  } catch (error) {
    line.textContent = error.message;
  }
};

(async () => {
  try {
    const response = await api('/api/canvases');
    const recent = await response.json();
    if (!navigation && recent.length) await openSaved(recent[0].id, 0);
  } catch (error) {
    if (!navigation) status(error.message, true);
  }
})();
