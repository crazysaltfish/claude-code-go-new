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
  const detail = $('app-detail');
  const tabs = new Map();
  const pendingStops = new Set();
  let apps = [];
  let active = null;
  let catalogGeneration = 0;
  let launchGeneration = 0;
  let detailGeneration = 0;

  const appPath = id => `/api/apps/${encodeURIComponent(id)}`;
  const appStatus = app => app.status === 'running' ? '运行中' : '已停止';

  function closeActions() {
    list.querySelector(':popover-open')?.hidePopover();
  }

  function showPanel(open) {
    panel.classList.toggle('hidden', !open);
    toggle.setAttribute('aria-expanded', String(open));
    if (open) $('task-panel').classList.add('hidden');
    else closeActions();
  }

  function closePanel() {
    showPanel(false);
    toggle.focus();
  }

  function select(tab) {
    if (tab) closeActions();
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
      const item = text('li', '', 'app-card');
      const button = text('button', '', 'app-card-open');
      button.type = 'button';
      const mark = text('span', app.builtin ? '⌘' : app.name.trim().slice(0, 1).toUpperCase(), 'app-mark');
      mark.setAttribute('aria-hidden', 'true');
      const copy = text('span', '', 'app-card-copy');
      const name = text('strong', app.name);
      copy.append(name, text('small', app.description || app.id));
      copy.append(text('span', appStatus(app), `app-state ${app.status}`));
      button.append(mark, copy);
      button.onclick = () => openApp(app);
      button.disabled = pendingStops.has(app.id);
      item.append(button, ...makeActions(app, button));
      return item;
    }));
  }

  function makeActions(app, openButton) {
    const trigger = text('button', '⋯', 'app-actions-trigger icon-button');
    trigger.type = 'button';
    trigger.id = `app-actions-trigger-${app.id}`;
    trigger.setAttribute('aria-label', `${app.name} 的操作`);
    trigger.title = '应用操作';
    const menu = text('div', '', 'app-actions');
    menu.id = `app-actions-${app.id}`;
    menu.setAttribute('popover', 'auto');
    trigger.setAttribute('popovertarget', menu.id);
    menu.addEventListener('toggle', event => {
      if (event.newState !== 'open') return;
      const anchor = trigger.getBoundingClientRect();
      menu.style.left = `${Math.max(8, anchor.right - menu.offsetWidth)}px`;
      menu.style.top = `${anchor.bottom + menu.offsetHeight + 12 <= window.innerHeight
        ? anchor.bottom + 5 : Math.max(8, anchor.top - menu.offsetHeight - 5)}px`;
    });
    const details = text('button', '查看详情');
    details.type = 'button';
    details.onclick = () => {
      menu.hidePopover();
      trigger.focus();
      showDetails(app);
    };
    const stop = text('button', '终止运行', 'app-stop-action');
    stop.type = 'button';
    stop.disabled = app.status !== 'running' || pendingStops.has(app.id);
    stop.onclick = () => {
      menu.hidePopover();
      trigger.focus();
      stop.disabled = openButton.disabled = true;
      terminateApp(app);
    };
    menu.append(details, stop);
    return [trigger, menu];
  }

  async function showDetails(app) {
    const generation = ++detailGeneration;
    $('app-detail-title').textContent = app.name;
    $('app-detail-description').textContent = '';
    $('app-detail-status').textContent = '正在读取应用详情…';
    const fields = $('app-detail-fields');
    fields.replaceChildren();
    detail.showModal();
    try {
      const current = await api(appPath(app.id));
      if (!detail.open || generation !== detailGeneration) return;
      $('app-detail-title').textContent = current.name;
      $('app-detail-description').textContent = current.description;
      $('app-detail-status').textContent = '';
      const rows = [
        ['运行状态', appStatus(current)],
        ['版本', current.version],
        ['App ID', current.id],
        ['安装来源', current.builtin ? '默认安装' : '本地安装'],
      ];
      if (current.capabilities?.includes('agent')) rows.push(['应用能力', '工作区 Agent']);
      fields.replaceChildren(...rows.flatMap(([label, value]) => [text('dt', label), text('dd', value)]));
    } catch (error) {
      if (detail.open && generation === detailGeneration) $('app-detail-status').textContent = error.message;
    }
  }

  async function terminateApp(app) {
    pendingStops.add(app.id);
    try {
      const launch = tabs.get(app.id)?.launch;
      if (launch) await launch.catch(() => {});
      const stopped = await api(`${appPath(app.id)}/stop`, {});
      const tab = tabs.get(app.id);
      if (tab) removeTab(tab);
      if (tab && active === tab) {
        ++launchGeneration;
        select(null);
      }
      apps = apps.map(item => item.id === stopped.id ? stopped : item);
      toast(`已终止 ${stopped.name} 的运行`);
    } catch (error) {
      toast(`无法终止 ${app.name}。${error.message}`);
    } finally {
      pendingStops.delete(app.id);
      if (!active && !panel.classList.contains('hidden') && !search.disabled) {
        const restoreFocus = document.activeElement?.id === `app-actions-trigger-${app.id}`;
        renderApps();
        if (restoreFocus) $(`app-actions-trigger-${app.id}`)?.focus();
      }
    }
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
    if (pendingStops.has(app.id)) return;
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
    const launch = tab.launch ??= api(`${appPath(app.id)}/launch`, {});
    try {
      const {url} = await launch;
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
    } finally {
      if (tab.launch === launch) tab.launch = null;
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
  catalog.addEventListener('scroll', closeActions);
  window.addEventListener('resize', closeActions);
  for (const id of ['tasks-button', 'toggle-tasks']) {
    $(id).addEventListener('click', () => {
      if (!$('task-panel').classList.contains('hidden')) showPanel(false);
    });
  }
})();
