'use strict';
document.getElementById('public-query').addEventListener('submit', async (event) => {
  event.preventDefault();
  const button = event.currentTarget.querySelector('button');
  if (button.disabled) return;
  const result = document.getElementById('query-result');
  const value = document.getElementById('query-value').value.trim();
  const kind = document.getElementById('query-kind').value;
  let path = '/api/network';
  if (kind === 'balance') {
    if (!/^0x[0-9a-fA-F]{40}$/.test(value)) { result.textContent = '請輸入有效的 0x 地址。'; return; }
    path = '/api/balance?address=' + encodeURIComponent(value);
  } else if (kind === 'transaction') {
    if (!/^0x[0-9a-fA-F]{64}$/.test(value)) { result.textContent = '請輸入有效的交易 Hash。'; return; }
    path = '/api/transactions/' + encodeURIComponent(value);
  }
  button.disabled = true;
  result.textContent = '查詢中…';
  try {
    const response = await fetch(document.getElementById('network').value + path, {signal: AbortSignal.timeout(15000)});
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || '查詢失敗，請稍後再試。');
    result.textContent = JSON.stringify(data, null, 2);
  } catch (error) {
    result.textContent = '無法取得結果：' + error.message;
  } finally {
    button.disabled = false;
  }
});
