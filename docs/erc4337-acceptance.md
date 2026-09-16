# ERC-4337 鏈上驗收

這個指令只驗證 Ethereum Sepolia、EntryPoint v0.6 與 SimpleAccount：第一次發送時部署帳戶，再轉 1 wei 給 owner。它沿用套件的 Builder、Signer 和 Bundler client，沒有接入錢包 UI。

## 2026-09-16 驗收通過

- EntryPoint：`0x5FF137D4b0FDCD49DcA30c7CF57E578a026d2789`
- Factory：`0x9406Cc6185a346906296840746125a0E44976454`
- Smart account：`0xFeD97208981509cba39e536BF9745cB0862593b8`
- UserOperation hash：`0x3ee5271ee7223cd7fde5c92caa09126e942c140e4719389a63e5e87d263e1fee`
- [水龍頭入金 0.012 測試 ETH](https://sepolia.etherscan.io/tx/0x6a08b8b52c6d05a29eb0b0b74d881a7b1512e2bee573e7ef9155f22cfe00025e)。這筆是入金，不是 UserOperation 的執行交易。

[執行交易](https://sepolia.etherscan.io/tx/0x12610361a80bcc374feb68ca3c6b3b2fa53bf0b76b2faf0e610693240f9e6c9b) 位於區塊 `11717225`。Bundler 收據 `success: true`，節點交易收據 `status: 0x1`。驗收指令已核對：

- 帳戶在前一區塊尚未部署，該區塊已存在程式碼，owner 與 EntryPoint 正確。
- `AccountDeployed`、`UserOperationEvent` 的 hash、帳戶、Factory、nonce 與成功狀態符合預期。
- owner `0x88E151447b16749509e28edAdC6093B03d0f8177` 的餘額從 0 增加到 1 wei。
- 收據區塊與節點的 canonical block hash 一致。

公開資料保留 Bundler 原始回覆與節點收據。Bundler 的內層 receipt 省略 status；指令會另查節點確認成功，保存時使用核對後的節點收據。

這份證據只涵蓋 Sepolia、EntryPoint v0.6 和上述 SimpleAccount，沒有驗證 v0.7、UI、Paymaster 或 finalized 狀態。

### 原操作為何失敗

原 hash `0x7c16a3f9662cfcf9381235410d18ebd675f56a87f5980f54e67f643bc8ab1266` 曾被 Bundler 接受，但[底層交易](https://sepolia.etherscan.io/tx/0xa3ec847a09acf70869643e166133abc587e1637f61d8c172e203278261702cb6) 的收據為 `status: 0x0`。以原交易參數在前一區塊重跑 `eth_call`，得到 `AA95 out of gas`：EntryPoint 的內層 gas 檢查未通過。先前只用較高 gas 的模擬，漏掉了這個條件。

這次改用 chain ID 形式的 Candide 公開端點，以零 gas 欄位要求估算，並在 `preVerificationGas` 加上一次 verification gas 的預留。這是本次 v0.6 部署驗收的保守處理，會增加實際付給 Bundler 的費用，不是通用的最低費用算法。總費用仍受 0.003 測試 ETH 上限限制。

修正操作沿用相同帳戶、nonce 0、收款人與金額，費率提高 20%。原操作及新操作分開保存；相同 nonce 避免兩筆都執行。一般重跑指令仍只重送已保存的操作，不會自動替換或加價。

## 重跑公開證據

不需要金鑰，也不會簽署、廣播或修改檔案：

```sh
go run ./cmd/accept-erc4337 --verify \
  --dir docs/evidence/erc4337-sepolia-v06 \
  --owner 0x88E151447b16749509e28edAdC6093B03d0f8177
```

指令會重算 UserOperation hash、驗證簽章，再核對 Bundler 收據、節點交易收據、canonical block、AccountDeployed／UserOperationEvent、帳戶 owner／EntryPoint，以及收款地址在該區塊增加的 1 wei。缺少資料或任一項不符就失敗；不把收據成功直接當成 finalized。

## 建立自己的驗收

```sh
go run ./cmd/accept-erc4337 --dir data/my-aa-test
# 將 Sepolia 測試 ETH 轉到指令列出的 sender。
go run ./cmd/accept-erc4337 --dir data/my-aa-test --send
go run ./cmd/accept-erc4337 --dir data/my-aa-test --verify
```

指令使用 Candide 公開 Bundler `https://api.candide.dev/public/v3/11155111`，單筆 gas 上限為 0.003 測試 ETH。簽署結果會先寫入 `operation.json` 並同步磁碟，再送出；重跑只使用同一筆操作。若等待逾時，可用 `--verify` 查詢，或用 `--send` 重送原操作，不會自動加價。

`data/` 內保存專用測試 keystore 與自動產生的密碼，兩者在同一目錄，只適合這個測試用途。不要提交這個目錄。公開資料只包含已送出的操作，不含金鑰或密碼。

## 依據

- [SimpleAccount v0.6](https://github.com/eth-infinitism/account-abstraction/blob/v0.6.0/contracts/samples/SimpleAccount.sol)：部署與簽章規則。
- [Candide Bundler API](https://docs.candide.dev/wallet/abstractionkit/bundler/)：公開端點與 RPC 方法。
- [permissionless SimpleAccount](https://github.com/pimlicolabs/permissionless.js/blob/main/packages/permissionless/accounts/simple/toSimpleSmartAccount.ts)：Factory 地址。
