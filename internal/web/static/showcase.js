'use strict';
(()=>{const button=document.getElementById('evidence-refresh'),list=document.getElementById('evidence');button.addEventListener('click',async()=>{if(button.disabled)return;button.disabled=true;list.textContent='正在查核…';try{const response=await fetch(document.getElementById('evidence-account').value+'/api/wallet/history',{signal:AbortSignal.timeout(15000)});const data=await response.json();if(!response.ok)throw new Error(data.error);list.replaceChildren();if(data.refreshError){const p=document.createElement('p');p.textContent=data.refreshError;list.append(p);}if(!data.transactions?.length)list.textContent='目前尚無可列出的本機發送紀錄；不以 Mock 測試代替鏈上交易證據。';for(const tx of data.transactions||[]){const p=document.createElement('p'),a=document.createElement('a');a.textContent=tx.hash;a.href='https://sepolia.etherscan.io/tx/'+encodeURIComponent(tx.hash);a.target='_blank';a.rel='noopener noreferrer';p.textContent=`${tx.action} · ${tx.amount} ${tx.symbol} · ${tx.state}${tx.finalized?' · finalized':''} `;p.append(a);list.append(p);}}catch(error){list.textContent='無法更新證據：'+error.message;}finally{button.disabled=false;}});})();

(async () => {
  const container = document.getElementById('verified-evidence');
  const text = (tag, value) => { const node = document.createElement(tag); node.textContent = value; return node; };
  try {
    const response = await fetch('/static/onchain-evidence.json');
    if (!response.ok) throw new Error('驗收紀錄無法載入');
    const data = await response.json();
    container.replaceChildren(text('p', `紀錄時間：${new Date(data.recordedAt).toLocaleString(document.documentElement.lang)} · ${new Set(data.transactions.map(tx => tx.network)).size} 個測試網`));
    const networks = {sepolia: 'Ethereum Sepolia', base: 'Base Sepolia', optimism: 'OP Sepolia', arbitrum: 'Arbitrum Sepolia', tron: 'TRON Shasta'};
    const operations = {native: '轉帳', base: '轉帳', optimism: '轉帳', arbitrum: '轉帳', wrap: 'ETH 換成 WETH', unwrap: 'WETH 換回 ETH', 'approve-usdc': '授權 USDC', 'approve-weth': '授權 WETH', swap: 'WETH → USDC', 'swap-back': 'USDC → WETH', 'usdc-transfer': 'USDC 轉帳', 'native-to-receiver': 'TRX 轉帳', token: '測試 USDT 轉帳'};
    for (const tx of data.transactions) {
      const row = text('article', ''); row.className = 'history-row';
      row.append(text('strong', `${networks[tx.network] || tx.network} · ${operations[tx.operation] || tx.operation} · ${tx.amount} ${tx.symbol}`));
      const link = text('a', tx.hash); link.href = tx.explorer; link.target = '_blank'; link.rel = 'noopener noreferrer';
      row.append(link, text('p', tx.receipt.finalized ? '驗收時已取得終局確認' : '驗收時已取得成功收據；終局性請重新查核'));
      container.append(row);
    }
    container.append(text('h3', '尚未完成的驗收與功能'));
    for (const limitation of data.remaining) container.append(text('p', limitation));
  } catch (error) { container.textContent = error.message; }
  try {
    const response = await fetch('/api/wallet/accounts');
    if (!response.ok) return;
    const accounts = await response.json(), select = document.getElementById('evidence-account');
    select.replaceChildren();
    for (const account of accounts) {
      const option = text('option', `${account.id ? '獨立帳戶' : '原有帳戶'} · ${account.address || '尚未建立'}`);
      option.value = account.id ? '/accounts/' + account.id : ''; select.append(option);
    }
  } catch { /* The public receipt snapshots remain useful without local accounts. */ }
})();
