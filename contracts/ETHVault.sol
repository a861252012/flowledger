// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/**
 * @title ETHVault
 * @notice Testnet-only ETH deposits and withdrawals; not audited for production.
 * Each address has an isolated balance. Follows Checks-Effects-Interactions (CEI)
 * and includes reentrancy protection.
 */
contract ETHVault {
    // Reentrancy guard status constants
    uint256 private constant _NOT_ENTERED = 1;
    uint256 private constant _ENTERED = 2;
    uint256 private _status;

    // Balance tracked per address
    mapping(address => uint256) private _balances;

    // Events for deposit and withdrawal logging
    event Deposited(address indexed account, uint256 amount);
    event Withdrawn(address indexed account, uint256 amount);

    // Custom errors for gas efficiency and clear reverts
    error ZeroDeposit();
    error ZeroWithdraw();
    error InsufficientBalance(uint256 requested, uint256 available);
    error TransferFailed();
    error ReentrantCall();

    modifier nonReentrant() {
        if (_status == _ENTERED) {
            revert ReentrantCall();
        }
        _status = _ENTERED;
        _;
        _status = _NOT_ENTERED;
    }

    constructor() {
        _status = _NOT_ENTERED;
    }

    /**
     * @notice Deposit ETH into the sender's isolated balance.
     */
    function deposit() external payable nonReentrant {
        if (msg.value == 0) {
            revert ZeroDeposit();
        }

        // Checks-Effects-Interactions: effect
        _balances[msg.sender] += msg.value;

        emit Deposited(msg.sender, msg.value);
    }

    /**
     * @notice Withdraw ETH from the sender's isolated balance.
     * @param amount The amount of wei to withdraw.
     */
    function withdraw(uint256 amount) external nonReentrant {
        if (amount == 0) {
            revert ZeroWithdraw();
        }

        uint256 balance = _balances[msg.sender];
        if (balance < amount) {
            revert InsufficientBalance(amount, balance);
        }

        // Checks-Effects-Interactions: update state before external transfer
        _balances[msg.sender] = balance - amount;

        emit Withdrawn(msg.sender, amount);

        // Interaction
        (bool success, ) = msg.sender.call{value: amount}("");
        if (!success) {
            revert TransferFailed();
        }
    }

    /**
     * @notice Query the vault balance for an account.
     * @param account The address to check.
     */
    function balanceOf(address account) external view returns (uint256) {
        return _balances[account];
    }
}
