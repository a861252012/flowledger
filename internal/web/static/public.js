'use strict';
(() => {
  // Reuse the wallet layout without loading private wallet controllers or data.
  $('wallet-dashboard').hidden = false;
  $('account-select').append(node('option', '公開唯讀展示 · 未選擇地址'));
  $('account-select').disabled = true;
  $('wallet-history').textContent = '錢包交易紀錄僅限擁有者查看；公開交易可至交易查核查詢。';
  $('token-list').textContent = '尚未選擇公開觀察地址。';
  $('history-search').disabled = true;
  $('wallet-balance-time').textContent = '請至地址餘額查詢公開地址；此處不顯示擁有者資產。';
  const allowedForms = new Set(['balance-form', 'transaction-form']);
  for (const form of document.querySelectorAll('form')) {
    if (allowedForms.has(form.id)) continue;
    for (const control of form.querySelectorAll('input,select,button,textarea')) control.disabled = true;
    form.append(node('p', '此功能僅限擁有者登入後使用。', 'field-hint'));
  }
  const allowedButtons = new Set(['menu-toggle', 'menu-close', 'menu-backdrop', 'theme-toggle', 'refresh-network', 'refresh-wallet']);
  for (const button of document.querySelectorAll('button')) {
    if (allowedButtons.has(button.id) || allowedForms.has(button.closest('form')?.id)) continue;
    button.disabled = true;
    button.title = '僅限擁有者登入後使用';
  }
  $('refresh-wallet').addEventListener('click', () => { location.hash = 'balance-panel'; $('address').focus(); });
  for (const option of $('network-select').options) {
    if (['/solana', '/tron'].includes(option.value)) option.disabled = true;
  }
})();
