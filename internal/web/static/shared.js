'use strict';
(() => {
  // Administration is also denied by SharedDemo on the server.
  for (const id of ['auto-scan-form', 'wallet-setup-form', 'keystore-form']) {
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
