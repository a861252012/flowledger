'use strict';
const assert = require('node:assert/strict');
const { setTimeout: delay } = require('node:timers/promises');
const { chromium } = require('playwright');

(async () => {
  const baseURL = process.env.WALLET_DEMO_URL;
  const revision = process.env.EXPECTED_REVISION;
  assert.ok(baseURL && revision, 'WALLET_DEMO_URL and EXPECTED_REVISION are required');

  // The VM polls releases after CI publishes; a healthy old revision is not success.
  const deadline = Date.now() + 10 * 60 * 1000;
  let deployed = false;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(new URL('/healthz', baseURL), {
        headers: { 'Cache-Control': 'no-cache' }, signal: AbortSignal.timeout(10000),
      });
      deployed = response.ok && response.headers.get('x-app-version') === revision && (await response.json()).status === 'ok';
      if (deployed) break;
    } catch { /* The application can briefly restart during deployment. */ }
    await delay(10000);
  }
  assert.ok(deployed, `server did not become healthy at revision ${revision}`);
  console.log(`PASS: deployed revision ${revision}`);

  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ baseURL, viewport: { width: 1280, height: 900 } });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    // Public-site checks never sign, broadcast, create an account, or claim funds.
    await page.goto('/#receive-panel');
    await page.locator('#wallet-dashboard').waitFor({ state: 'visible' });
    await page.waitForFunction(() => document.body.dataset.view === 'receive-panel');
    for (const view of ['send-panel', 'history-panel', 'vault-panel']) {
      await page.locator(`#app-sidebar a[href="#${view}"]`).click();
      await page.waitForFunction(expected => document.body.dataset.view === expected, view);
      assert.ok(await page.locator('#page-title').isVisible());
    }
    const vaultResponse = await context.request.get('/api/wallet/vault');
    assert.equal(vaultResponse.status(), 200);
    const vault = await vaultResponse.json();
    if (!vault.enabled) {
      await page.waitForFunction(() => document.querySelector('#vault-status').textContent === window.FlowI18n.t('存款箱尚未啟用。'));
      assert.equal(await page.locator('#vault-deposit').isDisabled(), true);
      assert.equal(await page.locator('#vault-withdraw').isDisabled(), true);
    } else {
      await page.waitForFunction(contract => document.querySelector('#vault-contract-view').textContent.toLowerCase().includes(contract.toLowerCase()), vault.contract);
      assert.equal(await page.locator('#vault-deposit').isEnabled(), true);
    }
    console.log(`PASS: EVM navigation; vault ${vault.enabled ? 'enabled' : 'disabled (no deployed contract configured)'}`);

    await page.locator('#app-sidebar a[href="#history-panel"]').click();
    for (const family of ['solana', 'tron']) {
      await page.locator('#network-select').selectOption(family === 'solana' ? '/solana' : '/tron/');
      await page.waitForURL(`**/${family}/#history-panel`);
      await page.waitForFunction(expected => document.body.dataset.walletFamily === expected && document.body.dataset.view === 'history-panel', family);
      const status = await context.request.get(`/${family}/api/status`);
      assert.equal(status.status(), 200);
      const wallet = await status.json();
      assert.equal(wallet.exists, true, `${family} demo wallet is missing`);
      assert.ok(wallet.csrfToken);
      const prefix = family === 'solana' ? 'sol' : 'tron';
      await page.locator(`#${prefix}-dashboard`).waitFor({ state: 'visible' });
      await page.waitForFunction(({ prefix, address }) => document.getElementById(prefix + '-address').textContent === address, { prefix, address: wallet.address });
      assert.ok(await page.locator('#history-panel').isVisible());
      console.log(`PASS: ${family} status and network switch preserve history panel`);
    }
    await page.setViewportSize({ width: 375, height: 812 });
    await page.locator('#menu-toggle').click();
    await page.locator('#app-sidebar a[href="#receive-panel"]').click();
    await page.waitForFunction(() => document.body.dataset.view === 'receive-panel' && !document.body.classList.contains('menu-open'));
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    assert.deepEqual(errors, []);
    const health = await context.request.get('/healthz');
    assert.equal(health.headers()['x-app-version'], revision, 'revision changed during browser verification');
    console.log('PASS: mobile navigation, no horizontal overflow, no page errors, revision unchanged');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
