'use strict';

const installed = document.querySelector('#installed');
const managementStatus = document.querySelector('#management-status');
const refresh = document.querySelector('#refresh');
let listGeneration = 0;

async function api(action, data) {
  const path = '/api/apps' + (action ? '/' + action : '');
  const response = await fetch(path, data && {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(data),
  });
  const result = await response.json();
  if (!response.ok) throw Error(result.error || '请求失败');
  return result;
}

function notice(element, message, error = false) {
  element.textContent = message;
  element.classList.toggle('error', error);
}

function operation(app, label, action) {
  const button = document.createElement('button');
  button.className = 'quiet';
  button.type = 'button';
  button.textContent = label;
  button.onclick = async () => {
    const buttons = button.parentElement.querySelectorAll('button');
    buttons.forEach(control => control.disabled = true);
    try {
      await api(action, {id: app.id});
      notice(managementStatus, `已${label} ${app.name}`);
      await loadApps();
    } catch (error) {
      notice(managementStatus, error.message, true);
    } finally {
      buttons.forEach(control => control.disabled = false);
    }
  };
  return button;
}

async function loadApps() {
  const generation = ++listGeneration;
  const apps = await api();
  if (generation !== listGeneration) return;
  installed.replaceChildren(...apps.map(app => {
    const row = document.createElement('li');
    const details = document.createElement('div');
    const name = document.createElement('strong');
    const meta = document.createElement('small');
    name.textContent = app.name;
    meta.textContent = [app.version, app.builtin ? '默认安装' : '本地安装', app.status === 'running' ? '运行中' : '已停止',
      ...(app.capabilities?.includes('agent') ? ['工作区 Agent'] : [])].join(' · ');
    details.append(name, meta);
    row.append(details);
    if (!app.builtin) {
      const actions = document.createElement('div');
      actions.className = 'actions';
      if (app.status === 'running') actions.append(operation(app, '停止', 'stop'));
      actions.append(operation(app, '卸载', 'remove'));
      row.append(actions);
    }
    return row;
  }));
}

function bindForm(selector, submit) {
  const form = document.querySelector(selector);
  const output = form.querySelector('output');
  form.addEventListener('submit', async event => {
    event.preventDefault();
    const buttons = form.querySelectorAll('button');
    buttons.forEach(button => button.disabled = true);
    notice(output, '正在处理…');
    try {
      const input = Object.fromEntries(new FormData(form));
      notice(output, await submit(input, event.submitter?.value));
    } catch (error) {
      notice(output, error.message, true);
    } finally {
      buttons.forEach(button => button.disabled = false);
    }
  });
}

bindForm('#install-form', async (input, action) => {
  action ||= 'inspect';
  const app = await api(action, {directory: input.directory.trim()});
  if (action === 'install') {
    await loadApps();
    return `已安装 ${app.name} ${app.version}。可从「扩展应用」打开。`;
  }
  const capability = app.capabilities?.includes('agent') ? '\n使用工作区 Agent 进行模型生成和公开检索。' : '';
  return `检查通过 · ${app.name} (${app.id}) · ${app.version}` + capability;
});

bindForm('#scaffold-form', async input => {
  const app = await api('scaffold', input);
  document.querySelector('#directory').value = app.directory;
  return `已创建 ${app.directory}\n在该目录中运行：\n${app.build}`;
});

refresh.onclick = () => loadApps().catch(error => notice(managementStatus, error.message, true));
refresh.click();
