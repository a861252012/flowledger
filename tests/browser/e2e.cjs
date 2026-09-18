// Local E2E: Chromium -> Go test server -> ETHVault bytecode on a simulated EVM.
// The local backend uses Sepolia's chain ID; this is not public Sepolia acceptance.
'use strict';
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const { spawn } = require('node:child_process');
const path = require('node:path');

async function runBrowserE2E(baseUrl, password, vaultContract) {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/*', route => new URL(route.request().url()).origin === new URL(baseUrl).origin ? route.continue() : route.abort());
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', err => errors.push(String(err)));

  async function confirmSend(action, amount) {
    const [response] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === '/api/wallet/send' && response.request().method() === 'POST'),
      page.locator('#confirm-send-button').click(),
    ]);
    assert.equal(response.status(), 200, 'local send API accepted the signed transaction');
    const transaction = await response.json();
    assert.match(transaction.hash, /^0x[0-9a-fA-F]{64}$/, 'send response contains a transaction hash');
    assert.equal(transaction.action, action, 'send response action matches the confirmed operation');
    assert.equal(transaction.amount, amount, 'send response amount matches the confirmed ETH amount');
    return transaction.hash;
  }

  async function verifyHistory(hash, action, amount, label) {
    const response = await context.request.get(baseUrl + '/api/wallet/history');
    assert.equal(response.status(), 200);
    const history = await response.json();
    assert.ok(!history.refreshError, 'history refreshed without an RPC error');
    const transaction = history.transactions.find(tx => tx.hash === hash);
    assert.ok(transaction, `${action} is present under its own hash`);
    assert.equal(transaction.action, action);
    assert.equal(transaction.amount, amount);
    assert.equal(transaction.state, 'succeeded', `${action} succeeded on the local simulated EVM`);
    await page.waitForFunction(({ hash, label, amount }) => {
      const row = [...document.querySelectorAll('#vault-history > p')].find(row =>
        [...row.querySelectorAll('a')].some(link => link.href.toLowerCase().endsWith('/tx/' + hash.toLowerCase())));
      const spans = row?.querySelectorAll('span');
      return row?.querySelector('strong')?.textContent === label &&
        spans[0]?.textContent === ` · ${amount} ETH · ` && spans[1]?.textContent === '鏈上執行成功';
    }, { hash, label, amount }, { timeout: 10000 });
  }

  try {
    // 1. Visit the vault panel on the local Go test server
    await page.goto(baseUrl + '/#vault-panel');
    await page.locator('#wallet-dashboard').waitFor({ state: 'visible', timeout: 15000 });

    // 2. Verify vault panel status & contract address
    await page.waitForFunction(() => !document.querySelector('#vault-preview').disabled, null, { timeout: 15000 });
    const contractLink = page.locator('#vault-contract-view a');
    assert.ok((await contractLink.getAttribute('href')).toLowerCase().includes(vaultContract.toLowerCase()), 'contract explorer link uses configured address');
    assert.equal((await contractLink.getAttribute('title')).toLowerCase(), vaultContract.toLowerCase());

    // Initial deposit balance should be 0 ETH
    const deposited = await page.locator('#vault-deposited-balance').textContent();
    assert.equal(deposited.trim(), '0 ETH', 'initial vault balance is exactly 0 ETH');

    // 3. Deposit 0.05 ETH on the local simulated EVM
    await page.locator('#vault-amount').fill('0.05');
    await page.locator('#vault-deposit').check();
    await page.locator('#vault-preview').click();

    // Verify confirmation modal
    await page.locator('#send-confirmation').waitFor({ state: 'visible', timeout: 10000 });
    const quoteText = await page.locator('#quote-details').textContent();
    assert.match(quoteText, /存入合約/);
    assert.equal(await page.locator('#confirm-title').textContent(), '確認存入合約');
    assert.ok(quoteText.toLowerCase().includes(vaultContract.toLowerCase()));

    // Fill password and sign & send on Go backend
    await page.locator('#send-password').fill(password);
    const depositHash = await confirmSend('vault_deposit', '0.05');
    await page.locator('#send-confirmation').waitFor({ state: 'hidden', timeout: 15000 });

    // 4. Verify balance and this deposit's successful history entry
    await page.waitForFunction(() => {
      const text = document.querySelector('#vault-deposited-balance')?.textContent || '';
      return text.trim() === '0.05 ETH';
    }, null, { timeout: 15000 });
    await page.waitForFunction(() => document.activeElement?.id === 'vault-history-section');
    await verifyHistory(depositHash, 'vault_deposit', '0.05', '存入合約');
    console.log('[PASS] Local simulated EVM: deposit 0.05 ETH succeeded; API and UI history match its transaction hash');

    // 5. Withdraw 0.02 ETH on the local simulated EVM
    await page.locator('#vault-amount').fill('0.02');
    await page.locator('#vault-withdraw').check();
    await page.locator('#vault-preview').click();

    // Verify withdrawal confirmation modal
    await page.locator('#send-confirmation').waitFor({ state: 'visible', timeout: 10000 });
    const withQuoteText = await page.locator('#quote-details').textContent();
    assert.match(withQuoteText, /取回錢包/);
    assert.equal(await page.locator('#confirm-title').textContent(), '確認取回錢包');

    // Sign and send withdrawal
    await page.locator('#send-password').fill(password);
    const withdrawHash = await confirmSend('vault_withdraw', '0.02');
    assert.notEqual(withdrawHash, depositHash, 'deposit and withdrawal are distinct transactions');
    await page.locator('#send-confirmation').waitFor({ state: 'hidden', timeout: 15000 });

    // 6. Verify remaining balance updated to 0.03 ETH in UI
    await page.waitForFunction(() => {
      const text = document.querySelector('#vault-deposited-balance')?.textContent || '';
      return text.trim() === '0.03 ETH';
    }, null, { timeout: 15000 });
    await page.waitForFunction(() => document.activeElement?.id === 'vault-history-section');
    await verifyHistory(withdrawHash, 'vault_withdraw', '0.02', '取回錢包');
    await verifyHistory(depositHash, 'vault_deposit', '0.05', '存入合約');
    console.log('[PASS] Local simulated EVM: withdrawal 0.02 ETH succeeded; API and UI history match its transaction hash; remaining 0.03 ETH');

    // 7. Adversarial Test: Over-withdrawal in UI
    // Remaining balance is 0.03 ETH, attempting to withdraw 0.5 ETH
    await page.locator('#vault-amount').fill('0.5');
    await page.locator('#vault-withdraw').check();
    await page.locator('#vault-preview').click();
    await page.waitForFunction(() => {
      const err = document.querySelector('#vault-error');
      return err && !err.hidden && err.textContent.includes('合約餘額不足，請減少取回金額');
    }, null, { timeout: 10000 });
    assert.equal(await page.locator('#send-confirmation').isVisible(), false, 'modal not opened on simulation failure');
    console.log('[PASS] Local simulated EVM: browser over-withdrawal rejected before send');

    // 8. Adversarial Test: Zero deposit rejected
    await page.locator('#vault-amount').fill('0');
    await page.locator('#vault-deposit').check();
    await page.locator('#vault-preview').click();
    await page.waitForFunction(() => {
      const err = document.querySelector('#vault-error');
      return (err && !err.hidden && err.textContent.includes('大於 0')) || !document.querySelector('#vault-form').checkValidity();
    }, null, { timeout: 10000 });
    console.log('[PASS] Local simulated EVM: browser zero-amount deposit rejected');

    // 9. Adversarial Test: Zero withdrawal rejected
    await page.locator('#vault-amount').fill('0');
    await page.locator('#vault-withdraw').check();
    await page.locator('#vault-preview').click();
    await page.waitForFunction(() => {
      const err = document.querySelector('#vault-error');
      return (err && !err.hidden && err.textContent.includes('大於 0')) || !document.querySelector('#vault-form').checkValidity();
    }, null, { timeout: 10000 });
    console.log('[PASS] Local simulated EVM: browser zero-amount withdrawal rejected');

    assert.deepEqual(errors, [], `Expected 0 page errors, but got ${errors.length}: ${errors.join('; ')}`);
    console.log('PASS: local browser + Go server + simulated EVM contract E2E; public Sepolia was not tested.');
  } finally {
    await context.close();
    await browser.close();
  }
}

(async () => {
  const baseUrl = process.env.E2E_BASE_URL;
  const password = process.env.E2E_PASSWORD || 'StrongE2ETestPass123!';
  const vaultContract = process.env.E2E_VAULT || '';

  if (baseUrl || vaultContract) {
    assert.ok(baseUrl && vaultContract, 'E2E_BASE_URL and E2E_VAULT must be provided together by the Go test harness');
    const target = new URL(baseUrl);
    assert.ok(target.protocol === 'http:' && ['127.0.0.1', '[::1]'].includes(target.hostname) && !target.username && !target.password,
      'Browser E2E requires a loopback Go test server; test:live is the read-only public-site check');
    assert.equal(process.env.E2E_BACKEND, 'simulated', 'Use the local Go simulated EVM harness');
    console.log('[ENV] Local simulated EVM; temporary test wallet; no public Sepolia transactions');
    await runBrowserE2E(baseUrl, password, vaultContract);
  } else {
    // Launch through Go E2E test runner
    console.log('[INFO] Launching local Go test server and simulated EVM ETHVault with -race...');
    const rootDir = path.resolve(__dirname, '../..');
    const child = spawn('go', ['test', '-race', '-count=1', '-v', '-run', '^TestE2EVaultBrowser$', './tests/e2e'], {
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
