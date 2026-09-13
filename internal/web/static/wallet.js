'use strict';
(() => {
  let walletState;
  let setupMode = 'create';
  let quote;
  let sending = false;
  const tokens = new Map();
  let activityPage = 1;
  let activityLoading = false;
  const actionLabels = {eth:"資產轉帳",transfer:"代幣轉帳",approve:"代幣授權",wrap:"ETH → WETH 包裝",unwrap:"WETH → ETH 解包",swap:"代幣兌換"};
  const stateLabels = { submitted: '已廣播，等待收錄', pending: '等待區塊收錄', broadcast_unknown: '廣播結果待確認', succeeded: '鏈上執行成功', reverted: '鏈上執行失敗', reorg_detected: '區塊變更，待確認', receipt_unavailable: '收據尚不可用' };

  async function walletRequest(path, body) {
    const options = { signal: AbortSignal.timeout(60000) };
    if (body !== undefined) {
      options.method = 'POST';
      options.headers = { 'Content-Type': 'application/json', 'X-Wallet-CSRF': walletState.csrfToken };
      options.body = JSON.stringify(body);
    }
    const response = await fetch(path, options);
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || '操作失敗，請稍後重試。');
    return data;
  }

  function showWalletError(id, error) {
    $(id).textContent = error.name === 'TimeoutError' ? '操作逾時，結果未知。請先更新錢包與交易紀錄，再決定是否重試。' : errorMessage(error);
    $(id).hidden = false;
  }

  async function refreshWallet() {
    if (!walletState?.exists) return;
    const button = $('refresh-wallet');
    if (button.disabled) return;
    button.disabled = true;
    $('check-funding').disabled = true;
    $('funding-status').textContent = '正在查詢 Sepolia 餘額…';
    $('wallet-error').hidden = true;
    $('wallet-balance-time').textContent = '正在查詢鏈上餘額…';
    const results = await Promise.allSettled([
      request(`/api/balance?address=${encodeURIComponent(walletState.address)}`),
      walletRequest('/api/wallet/history'),
    ]);
    if (results[0].status === 'fulfilled') {
      const balance = results[0].value;
      $('wallet-balance').replaceChildren(document.createTextNode(balance.eth + ' '), node('small', 'ETH'));
      $('wallet-balance-time').textContent = `區塊 ${balance.block} · ${time(balance.checkedAt)}`;
      $('funding-status').textContent = BigInt(balance.wei) > 0n
        ? `已查到 ${balance.eth} Sepolia ETH。可先預估費用，是否足夠仍以報價為準。`
        : '目前餘額為 0 Sepolia ETH。若剛領取，請稍後再查；只有 USDC 仍無法支付 gas。';
    } else {
      $('wallet-balance').replaceChildren(document.createTextNode('— '), node('small', 'ETH'));
      $('wallet-balance-time').textContent = '無法取得最新餘額';
      $('funding-status').textContent = '無法查到最新餘額，目前無法確認測試幣是否到帳，請稍後重試。';
      showWalletError('wallet-error', results[0].reason);
    }
    if (results[1].status === 'fulfilled') {
      renderHistory(results[1].value.transactions);
      if (results[1].value.refreshError) $('wallet-history').prepend(node('p', results[1].value.refreshError, 'error'));
    }
    else {
      $('wallet-history').replaceChildren(node('p', '無法更新紀錄，請稍後再試。', 'error'));
      showWalletError('wallet-error', results[1].reason);
    }
    await refreshTokens();
    button.disabled = false;
    $('check-funding').disabled = false;
  }

  function renderHistory(transactions) {
    const list = $('wallet-history');
    list.replaceChildren();
    if (!transactions?.length) { list.append(node('p', '尚無交易。第一筆轉帳會出現在這裡。', 'muted')); return; }
    for (const tx of transactions) {
      const row = node('article', '', 'history-row');
      const summary = node('div');
      summary.append(node('strong', actionLabels[tx.action] || '資產操作'), node('p', `${tx.action === 'approve' ? '被授權地址' : '收款人'} ${tx.to}`, 'mono'), explorer('tx', tx.hash), node('p', time(tx.createdAt)));
      const amount = node('div', '', 'history-amount');
      amount.append(node('strong', `${tx.amount} ${tx.symbol}`), document.createElement('br'), node('span', stateLabels[tx.state] || '狀態待確認', `state-badge${tx.state === 'succeeded' ? '' : tx.state === 'reverted' ? ' danger' : ' warning'}`));
      if (tx.confirmations) amount.append(node('p', `${tx.confirmations} 次確認`));
      if (tx.feeEth) amount.append(node('p', `實際費用 ${tx.feeEth} ETH`));
      if (tx.action !== 'eth' && tx.state === 'succeeded') summary.append(node('p', '收據顯示執行成功；代幣實際移動請核對合約紀錄。'));
      if (tx.state === 'broadcast_unknown' || tx.state === 'submitted' || tx.state === 'pending') {
        const retry = node('button', '重新廣播原交易', 'secondary');
        retry.type = 'button';
        retry.addEventListener('click', async () => {
          retry.disabled = true;
          try {
            const data = await walletRequest('/api/wallet/retry', { hash: tx.hash });
            showSent(data);
            await refreshWallet();
          } catch (error) { showWalletError('wallet-error', error); }
          finally { retry.disabled = false; }
        });
        summary.append(retry);
      }
      row.append(summary, amount);
      list.append(row);
    }
  }

  function showSent(data) {
    $('send-feedback').hidden = false;
    $('send-feedback').replaceChildren(node('strong', stateLabels[data.state] || '交易已記錄，結果待確認'), node('p', data.hash, 'mono'), explorer('tx', data.hash), node('p', '已記錄交易雜湊。更新交易紀錄以確認收錄結果；廣播成功不等於交易執行成功。'));
  }

  async function loadWallet() {
    $('wallet-loading').hidden = false;
    try {
      walletState = await walletRequest('/api/wallet');
      $('wallet-setup').hidden = walletState.exists;
      $('wallet-dashboard').hidden = !walletState.exists;
      $('nav-send').hidden = !walletState.exists;
      $('nav-history').hidden = !walletState.exists;
      $('nav-exchange').hidden = !walletState.exists;
      $('nav-activity').hidden = !walletState.exists;
      if (walletState.exists) {
        $('wallet-address').textContent = walletState.address;
        $('wallet-explorer').href = `https://sepolia.etherscan.io/address/${encodeURIComponent(walletState.address)}`;
        updateExchangeAction();
        await refreshWallet();
        await refreshActivity();
      }
    } catch (error) { showWalletError('wallet-error', error); }
    finally { $('wallet-loading').hidden = true; }
  }

  for (const mode of ['create', 'import']) {
    $(`choose-${mode}`).addEventListener('click', () => {
      setupMode = mode;
      for (const choice of ['create', 'import']) {
        $(`choose-${choice}`).classList.toggle('selected', choice === mode);
        $(`choose-${choice}`).setAttribute('aria-pressed', String(choice === mode));
      }
      $('mnemonic-input-group').hidden = mode !== 'import';
      $('import-mnemonic').required = mode === 'import';
      $('import-mnemonic').value = '';
      $('setup-submit').textContent = mode === 'import' ? '還原測試錢包' : '建立錢包';
      $('setup-error').hidden = true;
    });
  }

  $('wallet-setup-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    $('setup-error').hidden = true;
    const passwordLength = Array.from($('setup-password').value).length;
    if (passwordLength < 12 || passwordLength > 128) {
      showWalletError('setup-error', new Error('密碼長度必須介於 12 至 128 字元'));
      $('setup-password').focus();
      return;
    }
    if ($('setup-password').value !== $('confirm-password').value) {
      showWalletError('setup-error', new Error('兩次密碼不同，請重新確認。'));
      $('confirm-password').focus();
      return;
    }
    const mode = setupMode;
    const button = $('setup-submit');
    if (button.disabled) return;
    button.disabled = true;
    $('choose-create').disabled = true;
    $('choose-import').disabled = true;
    button.textContent = '正在加密金鑰，請稍候…';
    try {
      const body = { password: $('setup-password').value };
      if (mode === 'import') body.mnemonic = $('import-mnemonic').value.trim();
      const result = await walletRequest(`/api/wallet/${mode}`, body);
      form.reset();
      $('wallet-setup').hidden = true;
      if (result.mnemonic) {
        $('mnemonic-words').replaceChildren(...result.mnemonic.split(' ').map(word => node('li', word)));
        $('mnemonic-backup').hidden = false;
        $('backup-confirmed').focus();
      } else await loadWallet();
    } catch (error) {
      $('setup-password').value = '';
      $('confirm-password').value = '';
      $('import-mnemonic').value = '';
      showWalletError('setup-error', error);
    } finally {
      button.disabled = false;
      $('choose-create').disabled = false;
      $('choose-import').disabled = false;
      button.textContent = setupMode === 'import' ? '還原測試錢包' : '建立錢包';
    }
  });
  $('backup-confirmed').addEventListener('change', () => { $('finish-backup').disabled = !$('backup-confirmed').checked; });
  $('finish-backup').addEventListener('click', async () => {
    if (!$('backup-confirmed').checked) return;
    $('mnemonic-words').replaceChildren();
    $('mnemonic-backup').hidden = true;
    await loadWallet();
  });
  window.addEventListener('beforeunload', event => {
    if (!$('mnemonic-backup').hidden) { event.preventDefault(); event.returnValue = ''; }
  });

  $('copy-wallet-address').addEventListener('click', async () => {
    try { await navigator.clipboard.writeText(walletState.address); $('copy-wallet-address').textContent = '已複製'; }
    catch { showWalletError('wallet-error', new Error('無法複製，請選取上方完整地址手動複製。')); }
    setTimeout(() => { $('copy-wallet-address').textContent = '複製地址'; }, 2500);
  });
  $('claim-test-eth').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(walletState.address);
      $('faucet-status').textContent = '地址已複製。請在 Google 水龍頭貼上地址並申請；完成後按「我已領取，查詢餘額」。';
    } catch {
      $('faucet-status').textContent = '無法自動複製，請手動複製上方收款地址，在 Google 水龍頭貼上並申請。';
    }
  });
  $('refresh-wallet').addEventListener('click', refreshWallet);
  $('check-funding').addEventListener('click', refreshWallet);
  $('prepare-self-transfer').addEventListener('click', () => {
    if (!walletState?.exists || sending || $('send-confirmation').open) return;
    $('send-asset').value = 'eth';
    $('send-action').value = 'transfer';
    $('send-to').value = walletState.address;
    $('send-amount').value = '0.000001';
    $('send-error').hidden = true;
    updateSendAction();
    $('send-panel').scrollIntoView({block: 'start'});
    $('send-amount').focus({preventScroll: true});
  });
  $('prepare-first-wrap').addEventListener('click', () => {
    if (!walletState?.exists || sending || $('send-confirmation').open) return;
    $('exchange-action').value = 'wrap';
    $('exchange-amount').value = '0.000001';
    updateExchangeAction();
    $('exchange-panel').scrollIntoView({block: 'start'});
    $('exchange-amount').focus({preventScroll: true});
  });
  $('backup-form').addEventListener('submit', async event => {
    event.preventDefault();
    const button = event.currentTarget.querySelector('button');
    if (button.disabled) return;
    button.disabled = true;
    $('backup-feedback').textContent = '正在驗證密碼…';
    try {
      const data = await walletRequest('/api/wallet/backup', { password: $('backup-password').value });
      const url = URL.createObjectURL(new Blob([JSON.stringify(data)], { type: 'application/json' }));
      const link = node('a');
      link.href = url;
      link.download = `flowledger-sepolia-${walletState.address}.json`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      $('backup-feedback').textContent = '已要求瀏覽器下載加密備份，請妥善保存檔案與密碼。';
    } catch (error) { $('backup-feedback').textContent = errorMessage(error); }
    finally { $('backup-password').value = ''; button.disabled = false; }
  });

  $('token-form').addEventListener('submit', async event => {
    event.preventDefault();
    const button = event.currentTarget.querySelector('button');
    if (button.disabled) return;
    button.disabled = true;
    $('token-error').hidden = true;
    try {
      const token = await walletRequest('/api/wallet/token', { contract: $('token-contract').value.trim() });
      tokens.set(token.contract.toLowerCase(), token);
      renderTokens();
    } catch (error) { showWalletError('token-error', error); }
    finally { button.disabled = false; }
  });
  function renderTokens() {
      $('token-list').replaceChildren();
      const selected = $('send-asset').value;
      $('send-asset').replaceChildren(new Option('Sepolia ETH', 'eth'));
      for (const item of tokens.values()) {
        const row = node('div', '', 'token-row');
        row.append(node('strong', item.symbol), node('p', item.balance, 'token-balance mono'), node('p', item.contract, 'mono'));
        if (item.stale) row.append(node('p', '餘額未更新，顯示上次查詢結果。', 'error'));
        const raw = node('details');
        raw.append(node('summary', '代幣詳情'), details([['精度', item.decimals], ['最小單位餘額', item.balanceRaw]]), explorer('token', item.contract));
        row.append(raw);
        $('token-list').append(row);
        $('send-asset').append(new Option(`${item.symbol} · ${item.contract.slice(0, 8)}…${item.stale ? '（餘額未更新）' : ''}`, item.contract.toLowerCase()));
      }
      $('send-asset').value = [...$('send-asset').options].some(option => option.value === selected) ? selected : 'eth';
    updateSendAction();
  }
  function updateSendAction() {
    const token = $('send-asset').value !== 'eth';
    $('token-action-group').hidden = !token;
    const approve = token && $('send-action').value === 'approve';
    $('send-to-label').textContent = approve ? '被授權地址（spender）' : '收款地址';
    $('send-asset-hint').textContent = approve ? '只授權指定數量；輸入 0 代表撤銷。手續費以 Sepolia ETH 支付。' : '手續費另以 Sepolia ETH 支付。';
    if (tokens.get($('send-asset').value.toLowerCase())?.stale) $('send-asset-hint').textContent += ' 此代幣餘額未更新，預估費用時會重新查核。';
  }
  $('send-asset').addEventListener('change', updateSendAction);
  $('send-action').addEventListener('change', updateSendAction);
  $('send-form').addEventListener('submit', async event => {
    event.preventDefault();
    const button = event.currentTarget.querySelector('button');
    if (button.disabled) return;
    button.disabled = true;
    button.textContent = '正在預估與模擬…';
    $('send-error').hidden = true;
    quote = undefined;
    try {
      const token = $('send-asset').value !== 'eth';
      const data = await walletRequest('/api/wallet/quote', { action: token ? $('send-action').value : 'eth', to: $('send-to').value.trim(), amount: $('send-amount').value.trim(), contract: token ? $('send-asset').value : '' });
      openConfirmation(data);
    } catch (error) { showWalletError('send-error', error); }
    finally { button.disabled = false; button.textContent = '預估費用並核對'; }
  });
  function openConfirmation(data) {
    if ($('send-confirmation').open || sending) return;
      quote = data;
      const entries = [['操作', actionLabels[data.action] || '資產操作'], ['網路', 'Ethereum Sepolia'], ['發送地址', data.from], [data.action === 'approve' ? '被授權地址' : '收款地址', data.to], ['數量', `${data.amount} ${data.symbol}`], ['最高手續費', `${data.maxFeeEth} ETH`], ['最多扣除 ETH', `${data.totalEth} ETH`], ['報價有效至', time(data.expiresAt)]];
      if (data.contract) entries.splice(4, 0, ['代幣合約', data.contract]);
      if (data.exchange) {
        entries.push(['預估收到', `${data.exchange.expectedOut} ${data.exchange.symbolOut}`], ['最低收到', `${data.exchange.minimumOut} ${data.exchange.symbolOut}`], ['收到資產', data.exchange.tokenOut]);
        if (data.exchange.router) entries.push(['兌換合約', data.exchange.router], ['滑價', `${data.exchange.slippageBps / 100}%`], ['交易截止時間', time(data.exchange.deadline)]);
      }
      $('quote-details').replaceChildren(details(entries));
      const raw = document.createElement('details');
      raw.append(node('summary', '檢視實際簽署內容'), details([['Chain ID', 11155111], ['最小單位', data.amountRaw], ['方法', data.method], ['Nonce', data.nonce], ['Gas limit', data.gasLimit], ['Max fee / gas', `${data.maxFeePerGas} wei`], ['Priority fee / gas', `${data.maxPriorityFeePerGas} wei`]]), node('pre', data.data || '0x'));
      if (data.exchange?.pool) raw.append(details([['交易池', data.exchange.pool]]));
      $('quote-details').append(raw);
      $('approval-warning').hidden = data.action !== 'approve';
      $('confirm-error').hidden = true;
      $('send-password').value = '';
      $('send-confirmation').showModal();
      $('cancel-send').focus();
  }
  $('cancel-send').addEventListener('click', () => { if (!sending) $('send-confirmation').close(); });
  $('send-confirmation').addEventListener('cancel', event => { if (sending) event.preventDefault(); });
  $('send-confirmation').addEventListener('close', () => { $('send-password').value = ''; quote = undefined; });
  $('confirm-send-form').addEventListener('submit', async event => {
    event.preventDefault();
    if (sending || !quote) return;
    if (Date.now() >= new Date(quote.expiresAt).getTime()) { showWalletError('confirm-error', new Error('報價已過期，請取消並重新預估。')); return; }
    sending = true;
    $('confirm-send-button').disabled = true;
    $('cancel-send').disabled = true;
    $('confirm-send-button').textContent = '正在簽署與廣播…';
    $('confirm-error').hidden = true;
    try {
      const data = await walletRequest('/api/wallet/send', { quoteId: quote.id, password: $('send-password').value });
      $('send-confirmation').close();
      showSent(data);
      await refreshWallet();
      await refreshActivity();
    } catch (error) { showWalletError('confirm-error', error); }
    finally {
      $('send-password').value = '';
      sending = false;
      $('confirm-send-button').disabled = false;
      $('cancel-send').disabled = false;
      $('confirm-send-button').textContent = '簽署並送出至 Sepolia';
    }
  });
  async function refreshTokens() {
    const addresses = [...new Set([
      walletState.exchange?.weth, walletState.exchange?.usdc,
      ...tokens.keys(),
    ].filter(Boolean).map(address => address.toLowerCase()))];
    const results = await Promise.allSettled(addresses.map(contract => walletRequest('/api/wallet/token', {contract})));
    let failed = false;
    results.forEach((result, index) => {
      if (result.status === 'fulfilled') tokens.set(addresses[index].toLowerCase(), result.value);
      else {
        const token = tokens.get(addresses[index]);
        if (token) token.stale = true;
        failed = true;
      }
    });
    renderTokens();
    if (failed) showWalletError('token-error', new Error('部分代幣餘額無法更新，請稍後重試。'));
    else $('token-error').hidden = true;
  }

  function updateExchangeAction() {
    const action = $('exchange-action').value;
    const swap = action === 'weth-usdc' || action === 'usdc-weth';
    $('swap-options').hidden = !swap;
    $('exchange-error').hidden = true;
    const config = walletState?.exchange;
    if (!config) return;
    $('exchange-route').textContent = swap ? `Sepolia Router：${config.router}。收到的資產回到本錢包。` : `Sepolia WETH：${config.weth}。包裝／解包比率 1:1，另付 ETH gas。`;
  }
  $('exchange-action').addEventListener('change', updateExchangeAction);

  async function exchangeQuote(approval) {
    const button = approval === 'approve' ? $('swap-approve') : approval === 'revoke' ? $('swap-revoke') : $('exchange-submit');
    if (button.disabled) return;
    button.disabled = true;
    $('exchange-error').hidden = true;
    try {
      const action = $('exchange-action').value;
      const config = walletState.exchange;
      const input = action === 'usdc-weth' ? config.usdc : config.weth;
      const output = action === 'usdc-weth' ? config.weth : config.usdc;
      let payload;
      if (approval) payload = {action:'approve',to:config.router,contract:input,amount:approval === 'revoke' ? '0' : $('exchange-amount').value.trim()};
      else if (action === 'wrap' || action === 'unwrap') payload = {action,to:walletState.address,amount:$('exchange-amount').value.trim()};
      else payload = {action:'swap',to:walletState.address,contract:input,tokenOut:output,amount:$('exchange-amount').value.trim(),poolFee:Number($('swap-fee').value),slippageBps:Number($('swap-slippage').value)};
      const data = await walletRequest('/api/wallet/quote', payload);
      openConfirmation(data);
    } catch (error) { showWalletError('exchange-error', error); }
    finally { button.disabled = false; }
  }
  $('exchange-form').addEventListener('submit', event => {event.preventDefault();exchangeQuote();});
  $('swap-approve').addEventListener('click', () => exchangeQuote('approve'));
  $('swap-revoke').addEventListener('click', () => exchangeQuote('revoke'));

  function displayAsset(raw, asset) {
    const config = walletState.exchange;
    let decimals, symbol;
    if (asset === 'ETH') { decimals = 18; symbol = 'ETH'; }
    else if (asset.toLowerCase() === config.weth.toLowerCase()) { decimals = 18; symbol = 'WETH'; }
    else if (asset.toLowerCase() === config.usdc.toLowerCase()) { decimals = 6; symbol = '測試 USDC'; }
    else return `${raw} 最小單位 · ${asset}`;
    const negative = raw.startsWith('-');
    const digits = (negative ? raw.slice(1) : raw).padStart(decimals + 1, '0');
    const fraction = digits.slice(-decimals).replace(/0+$/, '');
    return `${negative ? '-' : ''}${digits.slice(0, -decimals)}${fraction ? '.' + fraction : ''} ${symbol}`;
  }

  async function refreshActivity() {
    if (activityLoading || !walletState?.exists) return;
    activityLoading = true;
    $('activity-refresh').disabled = true;
    $('activity-prev').disabled = true;
    $('activity-next').disabled = true;
    $('activity-error').hidden = true;
    $('activity-list').replaceChildren(node('p', '正在查核本頁鏈上收據…', 'muted'));
    $('activity-totals').replaceChildren();
    try {
      const data = await walletRequest(`/api/wallet/activity?page=${activityPage}`);
      $('activity-csv').href = `/api/wallet/activity?format=csv&page=${activityPage}`;
      $('activity-page').textContent = `第 ${data.page} / ${data.pages} 頁 · 已收錄 ${data.totalTransactions} 筆`;
      $('activity-prev').disabled = data.page <= 1;
      $('activity-next').disabled = data.page >= data.pages;
      $('activity-list').replaceChildren();
      if (!data.transactions.length) $('activity-list').append(node('p', '尚無收支。取得測試 ETH 後，可同步最近區塊或匯入收款交易。', 'muted'));
      if (data.incomplete) showWalletError('activity-error', new Error('部分交易尚未確認或無法查核，不列入本頁合計。'));
      for (const total of data.totals) {
        const box = node('div', '', 'activity-totals');
        box.append(node('strong', total.asset === 'ETH' ? '本頁 ETH 收支' : `本頁代幣 ${total.asset}`), node('p', `收入 ${displayAsset(total.receivedRaw,total.asset)} · 支出 ${displayAsset(total.sentRaw,total.asset)}`),node('p',`手續費 ${displayAsset(total.feeRaw,total.asset)} · 淨變動 ${displayAsset(total.netRaw,total.asset)}`));
        $('activity-totals').append(box);
      }
      const kinds = {receive:'收款',send:'付款',fee:'手續費'};
      for (const tx of data.transactions) {
        const row = node('article', '', 'history-row');
        row.append(node('strong', stateLabels[tx.state] || '無法查核'),explorer('tx',tx.hash));
        if (tx.blockTime) row.append(node('p',`${time(tx.blockTime)} · 區塊 ${tx.block}`));
        if (tx.error) row.append(node('p',tx.error,'error'));
        if (!tx.movements.length) row.append(node('p','尚無可列入的資產變動。授權操作本身不算代幣支出。'));
        for (const movement of tx.movements) {
          const item = node('div', '', `activity-move ${movement.kind}`);
          item.append(node('strong',`${kinds[movement.kind]} ${displayAsset(movement.raw,movement.asset)}`));
          if (movement.kind !== 'fee') item.append(node('p', `對方地址：${movement.counterparty}`));
          const raw = node('details');
          raw.append(node('summary', '鏈上紀錄詳情'), details([['紀錄來源', movement.evidence], ['原始數量', movement.raw], ['資產', movement.asset]]));
          item.append(raw);
          row.append(item);
        }
        $('activity-list').append(row);
      }
    } catch (error) { $('activity-list').replaceChildren(); showWalletError('activity-error',error); }
    finally { activityLoading = false; $('activity-refresh').disabled = false; }
  }
  $('activity-refresh').addEventListener('click',refreshActivity);
  $('activity-prev').addEventListener('click',()=>{if(!activityLoading){activityPage -= 1;refreshActivity();}});
  $('activity-next').addEventListener('click',()=>{if(!activityLoading){activityPage += 1;refreshActivity();}});
  $('activity-import-form').addEventListener('submit', async event => {
    event.preventDefault(); const button = event.currentTarget.querySelector('button');
    if (button.disabled) return;
    button.disabled = true; $('activity-error').hidden = true;
    try {
      const result = await walletRequest('/api/wallet/activity/import',{hash:$('activity-hash').value.trim()});
      $('activity-feedback').textContent = `已查核並收錄 ${result.hash}。重複匯入不會重複計帳。`;
      activityPage = 1; await refreshActivity();
    } catch (error) {showWalletError('activity-error',error);}
    finally {button.disabled = false;}
  });
  $('activity-sync-form').addEventListener('submit', async event => {
    event.preventDefault(); const button = event.currentTarget.querySelector('button');
    if (button.disabled) return;
    button.disabled = true; $('activity-error').hidden = true;
    $('activity-feedback').textContent = '正在掃描區塊，請稍候…';
    try {
      const raw = $('activity-from').value.trim();
      const from = raw ? Number(raw) : 0;
      if (!Number.isSafeInteger(from) || from < 0) throw new Error('請輸入有效的區塊號碼。');
      const result = await walletRequest('/api/wallet/activity/sync',{from,contracts:[...tokens.keys()]});
      $('activity-feedback').textContent = `已同步區塊 ${result.from}–${result.to}，新增 ${result.added} 筆。可保留下一個起始區塊繼續同步，或清空改查最近區塊。`;
      $('activity-from').value = String(result.to + 1);
      activityPage = 1; await refreshActivity();
    } catch (error) {$('activity-feedback').textContent = '';showWalletError('activity-error',error);}
    finally {button.disabled = false;}
  });

  loadWallet();
})();
