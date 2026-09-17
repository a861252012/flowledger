'use strict';
(() => {
  for (const id of ['activity-sync-settings', 'password-form', 'keystore-restore', 'wallet-setup', 'sol-create', 'sol-restore', 'sol-password-form', 'tron-create', 'tron-restore', 'tron-password-form']) {
    const el = document.getElementById(id);
    if (el) (el.closest('details') || el).setAttribute('data-shared-unavailable', '');
  }
  document.getElementById('show-archived-accounts')?.closest('label').setAttribute('data-shared-unavailable', '');
  document.querySelector('#account-list-view > .field-hint')?.setAttribute('data-shared-unavailable', '');
  // Administration is also denied by SharedDemo on the server.
  for (const id of ['auto-scan-form', 'wallet-setup-form', 'keystore-form', 'password-form', 'activity-sync-form', 'activity-import-form']) {
    const element = document.getElementById(id);
    if (!element) continue;
    if (element.matches('button,select')) element.disabled = true;
    for (const control of element.querySelectorAll('input,textarea,select,button')) control.disabled = true;
  }
  for (const id of ['stop-scan']) {
    const button = document.getElementById(id);
    if (button) button.disabled = true;
  }
  for (const id of ['sol-create', 'sol-restore', 'sol-password-form', 'tron-create', 'tron-restore', 'tron-password-form']) {
    const form = document.getElementById(id);
    if (form) for (const control of form.querySelectorAll('input,textarea,select,button')) control.disabled = true;
  }
})();
