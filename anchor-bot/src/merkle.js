const { MerkleTree } = require('merkletreejs');
const SHA256 = require('crypto-js/sha256');

const config = require('./config');
const logger = require('./logger');
const { submitToBabylon } = require('./babylon');

async function anchorBatch(batch) {
    const startHeight = batch[0].height;
    const endHeight = batch[batch.length - 1].height;
    const strataHeights = batch.map((item) => item.height);

    logger.info(`\n[🌳 MERKLE] Đang xử lý Block ${startHeight} -> ${endHeight}...`);

    const leaves = batch.map((item) => SHA256(item.hash));
    const tree = new MerkleTree(leaves, SHA256);
    const root = tree.getRoot().toString('hex');

    logger.info(`   🍃 Số lá: ${leaves.length}`);
    logger.info(`   🌳 Merkle Root: ${root}`);

    const memoPayload = `${config.BABYLON_MEMO_PREFIX}:${startHeight}:${endHeight}:${root}`;
    logger.info(`   📦 Payload: ${memoPayload}`);

    const txHash = await submitToBabylon(memoPayload);

    if (txHash) {
        logger.info(`   ✅ BATCH ANCHORED! Babylon Tx: ${txHash}`);
        logger.info('================================================');
        return {
            txHash,
            merkleRoot: root,
            leafCount: leaves.length,
            memoPayload,
            startHeight,
            endHeight,
            strataHeights,
        };
    }

    return null;
}

module.exports = {
    anchorBatch,
};
