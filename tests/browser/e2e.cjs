// Real End-to-End browser test connecting Chromium -> Go Backend Server -> EVM ETHVault Contract.
'use strict';
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const { spawn } = require('node:child_process');
const path = require('node:path');

async function runBrowserE2E(baseUrl, password, vaultContract) {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', err => errors.push(String(err)));

  try {
    // 1. Visit the vault panel on live Go server
    await page.goto(baseUrl + '/#vault-panel');
    await page.locator('#wallet-dashboard').waitFor({ state: 'visible', timeout: 15000 });

    // 2. Verify vault panel status & contract address
    await page.waitForFunction(() => !document.querySelector('#vault-deposit').disabled, null, { timeout: 15000 });
    const contractText = await page.locator('#vault-contract-view').textContent();
    assert.ok(contractText.toLowerCase().includes(vaultContract.toLowerCase()), `contract address visible in UI: ${contractText}`);

    // Initial deposit balance should be 0 ETH
    let deposited = await page.locator('#vault-deposited-balance').textContent();
    assert.ok(deposited.includes('0 ETH'), `initial vault balance is 0 ETH: ${deposited}`);

    // 3. Real Deposit: fill 0.05 ETH
    await page.locator('#vault-amount').fill('0.05');
    await page.locator('#vault-deposit').click();

    // Verify confirmation modal
    await page.locator('#send-confirmation').waitFor({ state: 'visible', timeout: 10000 });
    const quoteText = await page.locator('#quote-details').textContent();
    assert.match(quoteText, /存入存款箱/);
    assert.ok(quoteText.toLowerCase().includes(vaultContract.toLowerCase()));

    // Fill password and sign & send on Go backend
    await page.locator('#send-password').fill(password);
    await page.locator('#confirm-send-button').click();
    await page.locator('#send-confirmation').waitFor({ state: 'hidden', timeout: 15000 });

    // 4. Verify balance updated to 0.05 ETH in UI after on-chain execution
    await page.waitForFunction(() => {
      const text = document.querySelector('#vault-deposited-balance')?.textContent || '';
      return text.includes('0.05 ETH');
    }, null, { timeout: 15000 });
    console.log('[PASS] Real browser vault deposit confirmed on-chain and reflected in UI: 0.05 ETH');

    // 5. Real Withdrawal: fill 0.02 ETH
    await page.locator('#vault-amount').fill('0.02');
    await page.locator('#vault-withdraw').click();

    // Verify withdrawal confirmation modal
    await page.locator('#send-confirmation').waitFor({ state: 'visible', timeout: 10000 });
    const withQuoteText = await page.locator('#quote-details').textContent();
    assert.match(withQuoteText, /從存款箱提領/);

    // Sign and send withdrawal
    await page.locator('#send-password').fill(password);
    await page.locator('#confirm-send-button').click();
    await page.locator('#send-confirmation').waitFor({ state: 'hidden', timeout: 15000 });

    // 6. Verify remaining balance updated to 0.03 ETH in UI
    await page.waitForFunction(() => {
      const text = document.querySelector('#vault-deposited-balance')?.textContent || '';
      return text.includes('0.03 ETH');
    }, null, { timeout: 15000 });
    console.log('[PASS] Real browser vault withdrawal confirmed on-chain and reflected in UI: 0.03 ETH');

    // 7. Verify transaction history shows succeeded status
    await page.waitForFunction(() => {
      const text = document.querySelector('#vault-history')?.textContent || '';
      return text.includes('鏈上執行成功') || text.includes('succeeded');
    }, null, { timeout: 10000 });

    // 8. Adversarial Test: Over-withdrawal in UI
    // Remaining balance is 0.03 ETH, attempting to withdraw 0.5 ETH
    await page.locator('#vault-amount').fill('0.5');
    await page.locator('#vault-withdraw').click();
    await page.waitForFunction(() => {
      const err = document.querySelector('#vault-error');
      return err && !err.hidden && err.textContent.includes('存款箱餘額不足以提款');
    }, null, { timeout: 10000 });
    assert.equal(await page.locator('#send-confirmation').isVisible(), false, 'modal not opened on simulation failure');
    console.log('[PASS] Real browser over-withdrawal rejected before send');

    // 9. Adversarial Test: Zero deposit rejected
    await page.locator('#vault-amount').fill('0');
    await page.locator('#vault-deposit').click();
    await page.waitForFunction(() => {
      const err = document.querySelector('#vault-error');
      return (err && !err.hidden && err.textContent.includes('大於 0')) || !document.querySelector('#vault-form').checkValidity();
    }, null, { timeout: 10000 });
    console.log('[PASS] Real browser zero-amount deposit rejected');

    // 10. Adversarial Test: Zero withdrawal rejected
    await page.locator('#vault-amount').fill('0');
    await page.locator('#vault-withdraw').click();
    await page.waitForFunction(() => {
      const err = document.querySelector('#vault-error');
      return (err && !err.hidden && err.textContent.includes('大於 0')) || !document.querySelector('#vault-form').checkValidity();
    }, null, { timeout: 10000 });
    console.log('[PASS] Real browser zero-amount withdrawal rejected');

    assert.deepEqual(errors, [], `Expected 0 page errors, but got ${errors.length}: ${errors.join('; ')}`);
    console.log('PASS: E2E browser test successfully completed against live Go server and real EVM contract.');
  } finally {
    await context.close();
    await browser.close();
  }
}

(async () => {
  const baseUrl = process.env.E2E_BASE_URL;
  const password = process.env.E2E_PASSWORD || 'StrongE2ETestPass123!';
  const vaultContract = process.env.E2E_VAULT || '';

  if (baseUrl && vaultContract) {
    // Run directly against the provided server
    await runBrowserE2E(baseUrl, password, vaultContract);
  } else {
    // Launch through Go E2E test runner
    console.log('[INFO] Launching Go E2E test runner to provide live server and contract...');
    const rootDir = path.resolve(__dirname, '../..');
    const child = spawn('go', ['test', '-count=1', '-v', '-run', '^TestE2EVaultBrowser$', './tests/e2e'], {
      cwd: rootDir,
      stdio: 'inherit',
      env: { ...process.env, RUN_BROWSER_E2E: '1' },
    });
    child.on('error', err => {
      console.error('[FAIL] Failed to launch Go test runner:', err);
      process.exit(1);
    });
    child.on('exit', code => {
      process.exit(code ?? 1);
    });
  }
})().catch(err => {
  console.error('[FAIL]', err);
  process.exit(1);
});
