'use strict';
const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const solc = require('solc');

const sources = Object.fromEntries(['ETHVault.sol', 'ReentrancyAttacker.sol'].map(file => [file, {content: fs.readFileSync(path.join(__dirname, file), 'utf8')}]));
const input = {language:'Solidity', sources, settings:{evmVersion:'cancun', optimizer:{enabled:true, runs:200}, outputSelection:{'*':{'*':['abi','evm.bytecode.object']}}}};
const output = JSON.parse(solc.compile(JSON.stringify(input)));
for (const error of output.errors || []) console.error(error.formattedMessage);
if ((output.errors || []).some(error => error.severity === 'error')) process.exit(1);
const checking = process.argv.includes('--check');
for (const [file, contracts] of Object.entries(output.contracts)) {
  for (const [name, data] of Object.entries(contracts)) {
    if (!data.evm.bytecode.object) continue;
    const artifact = JSON.stringify({contractName:name, sourceFile:file, sourceHash:createHash('sha256').update(sources[file].content).digest('hex'), compiler:solc.version(), evmVersion:input.settings.evmVersion, abi:data.abi, bytecode:data.evm.bytecode.object}, null, 2) + '\n';
    const destination = path.join(__dirname, 'artifacts', name + '.json');
    if (checking) {
      if (!fs.existsSync(destination) || fs.readFileSync(destination, 'utf8') !== artifact) throw new Error('Stale artifact: ' + name + '; run npm run compile --prefix contracts');
    } else {
      fs.mkdirSync(path.dirname(destination), {recursive:true});
      fs.writeFileSync(destination, artifact);
    }
    console.log((checking ? 'Verified ' : 'Compiled ') + name);
  }
}
