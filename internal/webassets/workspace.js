'use strict';

(() => {
  const panel = $('project-panel');
  const tree = $('project-tree-list');
  const search = $('directory-search');
  const sort = $('directory-sort');
  const scopes = {
    session: {name: '会话目录', description: '当前会话的记录和子 Agent 文件。'},
    personal: {name: '个人目录', description: '本机 Claude 配置和数据文件。'},
    public: {name: '公共目录', description: '当前项目的文件，可在该项目的各个会话中浏览。'},
  };
  const date = new Intl.DateTimeFormat('zh-CN', {dateStyle: 'short', timeStyle: 'short'});
  const cache = new Map();
  const expanded = new Set(['session:']);
  let selection = {scope: 'session', path: ''};
  let sessionID = null;
  let opener = null;

  const key = (scope, path) => `${scope}:${path}`;
  const join = (path, name) => path ? path + '/' + name : name;
  const visible = () => !panel.classList.contains('hidden');

  function load(scope, path, refresh = false) {
    if (scope === 'session' && !sessionID) return;
    const id = key(scope, path);
    if (!refresh && cache.has(id)) return;
    const record = {};
    cache.set(id, record);
    const query = new URLSearchParams({path});
    if (scope === 'session') query.set('chat', sessionID);
    api(`/api/workspace/${scope}?${query}`).then(data => {
      record.data = data;
    }).catch(error => {
      record.error = error.message;
    }).finally(() => {
      if (cache.get(id) !== record || !visible()) return;
      renderTree();
      if (id === key(selection.scope, selection.path)) renderFiles();
    });
  }

  function reveal(scope, path) {
    let prefix = '';
    expanded.add(key(scope, prefix));
    for (const part of path ? path.split('/') : []) {
      prefix = join(prefix, part);
      expanded.add(key(scope, prefix));
      load(scope, prefix);
    }
  }

  function selectDirectory(scope, path, expand = true) {
    selection = {scope, path};
    search.value = '';
    if (expand) reveal(scope, path);
    load(scope, path);
    render();
  }

  function treeItem(scope, path, name, kind) {
    const item = text('li', '', 'project-tree-item');
    const id = key(scope, path);
    const directory = kind === 'directory';
    const row = text(directory ? 'button' : 'span', '', 'project-tree-row');
    const disclosure = text('span', '', 'directory-chevron');
    if (directory) disclosure.append(uiIcon('chevron'));
    row.append(disclosure, uiIcon(directory ? 'folder' : kind), text('span', name, 'tree-name'));
    row.title = name;
    if (directory) {
      row.type = 'button';
      row.dataset.directory = id;
      row.setAttribute('aria-expanded', String(expanded.has(id)));
      if (selection.scope === scope && selection.path === path) row.setAttribute('aria-current', 'true');
      row.onclick = () => {
        if (expanded.has(id)) {
          expanded.delete(id);
          selectDirectory(scope, path, false);
        } else {
          selectDirectory(scope, path);
        }
        findTreeRow(id)?.focus();
      };
    }
    item.append(row);
    if (!directory || !expanded.has(id)) return item;
    const children = text('ul', '', 'project-tree-children');
    const record = cache.get(id);
    if (record?.data) {
      for (const entry of record.data.entries) {
        children.append(treeItem(scope, join(path, entry.name), entry.name, entry.kind));
      }
    } else if (record?.error) {
      children.append(text('li', record.error, 'directory-tree-note'));
    }
    if (children.childElementCount) item.append(children);
    return item;
  }

  function findTreeRow(id) {
    return Array.from(tree.querySelectorAll('[data-directory]')).find(row => row.dataset.directory === id);
  }

  function renderTree() {
    const focused = tree.contains(document.activeElement) ? document.activeElement.dataset.directory : null;
    tree.replaceChildren(...Object.entries(scopes).map(([scope, info]) => treeItem(scope, '', info.name, 'directory')));
    if (focused) findTreeRow(focused)?.focus({preventScroll: true});
  }

  function renderDirectory() {
    const {scope, path} = selection;
    const meta = scopes[scope];
    const parts = path ? path.split('/') : [];
    $('directory-title').textContent = parts.at(-1) || meta.name;
    $('directory-description').textContent = meta.description;
    const crumbs = $('directory-crumbs');
    crumbs.replaceChildren();
    if (parts.length) {
      [meta.name, ...parts].forEach((name, index) => {
        if (index) crumbs.append(uiIcon('chevron'));
        if (index === parts.length) {
          crumbs.append(text('span', name));
          return;
        }
        const button = text('button', name);
        button.type = 'button';
        button.onclick = () => selectDirectory(scope, parts.slice(0, index).join('/'));
        crumbs.append(button);
      });
    }
    renderFiles();
  }

  function renderFiles() {
    const {scope, path} = selection;
    const record = cache.get(key(scope, path));
    const pathLabel = $('directory-path');
    pathLabel.title = pathLabel.textContent = record?.data?.path || '';
    const results = $('directory-files');
    results.replaceChildren();
    $('directory-table').hidden = !record?.data;
    const status = $('directory-status');
    if (!record?.data) {
      status.textContent = scope === 'session' && !sessionID ? '正在连接当前会话…' : record?.error || '正在加载目录…';
      return;
    }
    const term = search.value.trim().toLocaleLowerCase();
    const entries = record.data.entries.filter(entry => entry.name.toLocaleLowerCase().includes(term));
    entries.sort((a, b) => Number(b.kind === 'directory') - Number(a.kind === 'directory') ||
      (sort.value === 'modified' ? Date.parse(b.modified) - Date.parse(a.modified) : a.name.localeCompare(b.name, 'zh-CN')));
    for (const entry of entries) {
      const row = document.createElement('tr');
      const name = document.createElement('td');
      const label = text(entry.kind === 'directory' ? 'button' : 'span', '', 'directory-entry');
      label.append(uiIcon(entry.kind === 'directory' ? 'folder' : entry.kind), text('span', entry.name));
      label.title = entry.name;
      if (entry.kind === 'directory') {
        label.type = 'button';
        label.onclick = () => selectDirectory(scope, join(path, entry.name));
      }
      name.append(label);
      row.append(name, text('td', date.format(new Date(entry.modified))));
      results.append(row);
    }
    status.textContent = entries.length ? '' : term ? '没有匹配的文件或文件夹。' : '此目录为空。';
  }

  function render() {
    renderTree();
    renderDirectory();
  }

  function syncSession() {
    if (sessionID === state.id) return;
    sessionID = state.id;
    for (const id of cache.keys()) if (id.startsWith('session:')) cache.delete(id);
    for (const id of expanded) if (id.startsWith('session:')) expanded.delete(id);
    if (selection.scope === 'session') {
      selection.path = '';
      search.value = '';
      expanded.add('session:');
    }
    if (visible()) {
      if (expanded.has('session:')) load('session', '');
      render();
    }
  }

  function show(open) {
    panel.classList.toggle('hidden', !open);
    for (const id of ['toggle-project', 'project-button']) $(id).setAttribute('aria-expanded', String(open));
    if (!open) return;
    showSidePanel('project');
    syncSession();
    load(selection.scope, selection.path, true);
    render();
  }

  for (const id of ['toggle-project', 'project-button']) {
    $(id).onclick = () => {
      opener = $(id);
      if (!visible() && id === 'project-button' && innerWidth <= 760) {
        $('sidebar').classList.add('collapsed');
        opener = $('toggle-project');
      }
      show(!visible());
    };
  }
  $('close-project').onclick = () => {
    show(false);
    opener?.focus();
  };
  $('reload-project').onclick = () => {
    load(selection.scope, selection.path, true);
    renderFiles();
  };
  search.oninput = sort.onchange = renderFiles;
  document.addEventListener('web-chat-changed', syncSession);
  document.addEventListener('web-side-panel', event => {
    if (event.detail !== 'project') show(false);
  });
})();
