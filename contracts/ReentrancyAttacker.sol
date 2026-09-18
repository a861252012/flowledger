// SPDX-License-Identifier: MIT
pragma solidity 0.8.37;

interface IVault {
    function deposit() external payable;
    function withdraw(uint256 amount) external;
    function balanceOf(address account) external view returns (uint256);
}

contract ReentrancyAttacker {
    IVault public immutable vault;
    bool public attackAttempted;
    bool public attackSucceeded;
    string public revertReason;

    constructor(address _vault) {
        vault = IVault(_vault);
    }

    function deposit() external payable {
        vault.deposit{value: msg.value}();
    }

    function attack(uint256 amount) external {
        attackAttempted = false;
        attackSucceeded = false;
        vault.withdraw(amount);
    }

    receive() external payable {
        if (!attackAttempted) {
            attackAttempted = true;
            // Attempt reentrancy call to withdraw
            try vault.withdraw(msg.value) {
                attackSucceeded = true;
            } catch Error(string memory reason) {
                revertReason = reason;
            } catch (bytes memory) {
                revertReason = "custom error";
            }
        }
    }
}

contract RejectingReceiver {
    IVault public immutable vault;

    constructor(address _vault) {
        vault = IVault(_vault);
    }

    function deposit() external payable {
        vault.deposit{value: msg.value}();
    }

    function withdraw(uint256 amount) external {
        vault.withdraw(amount);
    }

    // Reverts on receiving ETH
    receive() external payable {
        revert("rejecting ETH transfers");
    }
}
