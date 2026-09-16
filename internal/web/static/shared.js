'use strict';
(() => {
  for (const id of ['activity-sync-settings', 'password-form', 'keystore-restore', 'wallet-setup', 'sol-create', 'sol-restore', 'sol-password-form', 'tron-create', 'tron-restore', 'tron-password-form']) {
    const el = document.getElementById(id);
    if (el) (el.closest('details') || el).setAttribute('data-shared-unavailable', '');
  }
  document.getElementById('show-archived-accounts')?.closest('label').setAttribute('data-shared-unavailable', '');
  document.querySelector('#account-list-view > .field-hint')?.setAttribute('data-shared-unavailable', '');
  for (const id of ['claim-native', 'claim-usdc', 'sol-airdrop', 'tron-claim']) document.getElementById(id)?.setAttribute('data-shared-unavailable', '');
  const funding = document.getElementById('test-funding-panel');
  if (funding) {
    const family = document.body.dataset.walletFamily;
    const links = family === 'solana' ? [['Solana Devnet Faucet ↗','https://faucet.solana.com/']] : family === 'tron' ? [['Shasta Faucet ↗','https://shasta.tronex.io/join/getJoinPage']] : [['Ethereum Sepolia Faucet ↗','https://cloud.google.com/application/web3/faucet/ethereum/sepolia'],['Circle Test USDC ↗','https://faucet.circle.com/']];
    const copy = document.createElement('p'); copy.textContent = '請使用符合目前網路的外部水龍頭；領取後重新整理餘額。';
    const group = document.createElement('div'); group.className = 'feature-actions';
    for (const [name, url] of links) { const a = document.createElement('a'); a.textContent = name; a.href = url; a.target = '_blank'; a.rel = 'noopener noreferrer'; group.append(a); }
    if (family === 'evm') {
      const slug = {421614:'arbitrum-sepolia',84532:'base-sepolia',11155420:'optimism-sepolia',80002:'polygon-amoy'}[networkID];
      if (slug) { group.replaceChildren(); const a=document.createElement('a'); a.textContent=networkName + ' Faucet ↗'; a.href='https://www.alchemy.com/faucets/' + slug; a.target='_blank'; a.rel='noopener noreferrer'; group.append(a); }
      funding.querySelector('.field-hint').textContent = '外部服務可能要求登入或符合領取資格；請勿提供助記詞或私鑰。';
      document.getElementById('test-funding-status').textContent = copy.textContent;
    }
    funding.append(group);
    for (const id of ['sol-funding-status','tron-funding-status']) { const el=document.getElementById(id); if(el) el.textContent=copy.textContent; }
  }
  // Administration is also denied by SharedDemo on the server.
  for (const id of ['auto-scan-form', 'wallet-setup-form', 'keystore-form', 'password-form', 'activity-sync-form', 'activity-import-form']) {
    const element = document.getElementById(id);
    if (!element) continue;
    if (element.matches('button,select')) element.disabled = true;
    for (const control of element.querySelectorAll('input,textarea,select,button')) control.disabled = true;
  }
  for (const id of ['stop-scan', 'claim-native', 'claim-usdc', 'sol-airdrop', 'tron-claim']) {
    const button = document.getElementById(id);
    if (button) button.disabled = true;
  }
  for (const id of ['sol-create', 'sol-restore', 'sol-password-form', 'tron-create', 'tron-restore', 'tron-password-form']) {
    const form = document.getElementById(id);
    if (form) for (const control of form.querySelectorAll('input,textarea,select,button')) control.disabled = true;
  }
})();
