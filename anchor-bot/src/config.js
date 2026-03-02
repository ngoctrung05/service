require('dotenv').config();

function getEnvInt(key, fallback) {
    const raw = process.env[key];
    if (!raw) {
        return fallback;
    }

    const parsed = Number.parseInt(raw, 10);
    if (Number.isNaN(parsed)) {
        return fallback;
    }

    return parsed;
}

const config = {
    STRATA_RPC: process.env.STRATA_RPC || 'http://131.153.224.169:26757',
    BABYLON_RPC: process.env.BABYLON_RPC || 'https://babylon-testnet-rpc.nodes.guru',
    BABYLON_DENOM: process.env.BABYLON_DENOM || 'ubbn',
    BATCH_SIZE: getEnvInt('BATCH_SIZE', 20),
    POLL_INTERVAL_MS: getEnvInt('POLL_INTERVAL_MS', 2000),
    BABYLON_MEMO_PREFIX: process.env.BABYLON_MEMO_PREFIX || 'ENGRAM',
    MNEMONIC: process.env.BABYLON_MNEMONIC ? process.env.BABYLON_MNEMONIC.trim() : '',
};

module.exports = config;
