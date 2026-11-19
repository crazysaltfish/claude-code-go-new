'use strict';

(() => {
  const panel = $('app-panel');
  const toggle = $('toggle-apps');
  const catalog = $('app-catalog');
  const catalogStatus = $('app-catalog-status');
  const launchStatus = $('app-launch-status');
  const frames = $('app-frames');
  const list = $('app-list');
  const search = $('app-search');
  const tabs = new Map();
  let apps = [];
  let active = null;
  let catalogGeneration = 0;
  let launchGeneration = 0;

  function showPanel(open) {
    panel.classList.toggle('hidden', !open);
    toggle.setAttribute('aria-expanded', String(open));
    if (open) $('task-panel').classList.add('hidden');
  }

  function closePanel() {
    showPanel(false);
    toggle.focus();
  }

  function select(tab) {
    active = tab;
    catalog.classList.toggle('hidden', Boolean(tab));
    frames.classList.toggle('hidden', !tab);
    launchStatus.classList.add('hidden');
    launchStatus.classList.remove('app-error');
    $('app-catalog-tab').setAttribute('aria-current', tab ? 'false' : 'page');
    for (const item of tabs.values()) {
      item.button.setAttribute('aria-current', item === tab ? 'page' : 'false');
      item.frame.classList.toggle('hidden', item !== tab);
    }
  }

  async function showCatalog() {
    ++launchGeneration;
    select(null);
    const generation = ++catalogGeneration;
    search.disabled = true;
    list.replaceChildren();
    catalogStatus.textContent = '正在读取已安装应用…';
    try {
      const data = await api('/api/apps');
      if (generation !== catalogGeneration) return;
      apps = data;
      const installed = new Map(apps.map(app => [app.id, app]));
      for (const tab of tabs.values()) {
        const current = installed.get(tab.app.id);
        if (current) tab.app = current;
        else removeTab(tab);
      }
      renderApps();
    } catch (error) {
      if (generation === catalogGeneration) catalogStatus.textContent = error.message;
    } finally {
      if (generation === catalogGeneration) search.disabled = false;
    }
  }

  function renderApps() {
    const query = search.value.trim().toLowerCase();
    const filtered = apps.filter(app => [app.name, app.id, app.description].some(value => value.toLowerCase().includes(query)));
    catalogStatus.textContent = filtered.length ? '' : '未找到匹配的应用。';
    list.replaceChildren(...filtered.map(app => {
      const item = document.createElement('li');
      const button = text('button', '', 'app-card');
      button.type = 'button';
      const mark = text('span', app.builtin ? '⌘' : app.name.trim().slice(0, 1).toUpperCase(), 'app-mark');
      mark.setAttribute('aria-hidden', 'true');
      const copy = text('span', '', 'app-card-copy');
      const name = text('strong', app.name);
      copy.append(name, text('small', app.description || app.id));
      const arrow = text('span', '›', 'app-card-arrow');
      arrow.setAttribute('aria-hidden', 'true');
      button.append(mark, copy, arrow);
      button.onclick = () => openApp(app);
      item.append(button);
      return item;
    }));
  }

  function makeTab(app) {
    const group = text('div', '', 'app-tab-group');
    const button = text('button', app.name, 'app-tab');
    const close = text('button', '×', 'app-tab-close');
    button.type = close.type = 'button';
    close.setAttribute('aria-label', `关闭 ${app.name}`);
    const frame = document.createElement('iframe');
    frame.className = 'app-frame hidden';
    frame.title = app.name;
    frame.setAttribute('sandbox', 'allow-scripts allow-same-origin allow-forms allow-popups');
    frame.referrerPolicy = 'no-referrer';
    const tab = {app, button, close, group, frame, url: ''};
    button.onclick = () => openApp(tab.app);
    close.onclick = () => {
      const wasActive = active === tab;
      removeTab(tab);
      if (wasActive) {
        showCatalog();
        $('app-catalog-tab').focus();
      }
    };
    group.append(button, close);
    $('app-tabs').append(group);
    frames.append(frame);
    tabs.set(app.id, tab);
    return tab;
  }

  function removeTab(tab) {
    tabs.delete(tab.app.id);
    tab.group.remove();
    tab.frame.remove();
  }

  async function openApp(app, reload = false) {
    const tab = tabs.get(app.id) || makeTab(app);
    tab.app = app;
    tab.button.textContent = tab.frame.title = app.name;
    tab.close.setAttribute('aria-label', `关闭 ${app.name}`);
    const generation = ++launchGeneration;
    select(tab);
    if (!tab.url || reload) {
      tab.frame.classList.add('hidden');
      launchStatus.textContent = `正在启动 ${app.name}…`;
      launchStatus.classList.remove('hidden');
      frames.classList.add('hidden');
    }
    try {
      const {url} = await api(`/api/apps/${encodeURIComponent(app.id)}/launch`, {});
      if (generation !== launchGeneration) return;
      if (tab.url !== url || reload) tab.frame.src = url;
      tab.url = url;
      tab.frame.classList.remove('hidden');
      frames.classList.remove('hidden');
      launchStatus.classList.add('hidden');
    } catch (error) {
      if (generation !== launchGeneration) return;
      frames.classList.add('hidden');
      launchStatus.textContent = `无法启动 ${app.name}。${error.message}`;
      launchStatus.classList.add('app-error');
      launchStatus.classList.remove('hidden');
    }
  }

  toggle.onclick = () => {
    if (!panel.classList.contains('hidden')) {
      closePanel();
      return;
    }
    showPanel(true);
    if (active) openApp(active.app);
    else showCatalog();
  };
  $('app-catalog-tab').onclick = showCatalog;
  $('close-app-panel').onclick = closePanel;
  $('reload-app').onclick = () => active ? openApp(active.app, true) : showCatalog();
  $('expand-app').onclick = event => {
    const control = event.currentTarget;
    const expanded = panel.classList.toggle('expanded');
    control.textContent = expanded ? '⤡' : '⤢';
    control.title = expanded ? '还原应用面板' : '扩大应用面板';
    control.setAttribute('aria-label', control.title);
    control.setAttribute('aria-pressed', String(expanded));
  };
  search.oninput = renderApps;
  for (const id of ['tasks-button', 'toggle-tasks']) {
    $(id).addEventListener('click', () => {
      if (!$('task-panel').classList.contains('hidden')) showPanel(false);
    });
  }
})();
