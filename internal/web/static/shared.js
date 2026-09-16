'use strict';
(() => {
  // Administration is also denied by SharedDemo on the server.
  for (const id of ['manage-accounts', 'account-select', 'auto-scan-form', 'wallet-setup-form', 'keystore-form']) {
    const element = document.getElementById(id);
    if (!element) continue;
    if (element.matches('button,select')) element.disabled = true;
    for (const control of element.querySelectorAll('input,textarea,select,button')) control.disabled = true;
  }
  for (const id of ['stop-scan', 'claim-native', 'claim-usdc']) document.getElementById(id).disabled = true;
  for (const option of document.getElementById('network-select').options) {
    if (['/solana','/tron'].includes(option.value)) option.disabled = true;
  }
})();
