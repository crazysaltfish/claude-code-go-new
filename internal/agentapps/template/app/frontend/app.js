'use strict';

const form = document.querySelector('form');
const output = document.querySelector('output');
form.addEventListener('submit', async event => {
  event.preventDefault();
  try {
    const response = await fetch('/api/echo', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({message: document.querySelector('#message').value}),
    });
    if (!response.ok) throw Error(`服务返回 ${response.status}`);
    output.textContent = (await response.json()).message;
  } catch (error) {
    output.textContent = error.message;
  }
});
