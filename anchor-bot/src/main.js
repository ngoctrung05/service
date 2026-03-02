const config = require('./config');
const logger = require('./logger');
const { getStrataBlock } = require('./strata');
const { anchorBatch } = require('./merkle');
const {
    getEpochFilePath,
    getBabylonAnchorFilePath,
    getBabylonPendingFilePath,
    getNextEpoch,
    persistHeightEpoch,
    persistBabylonPending,
    persistBabylonAnchor,
} = require('./epoch-store');

function validateRuntimeConfig() {
    if (!config.MNEMONIC) {
        logger.error('❌ LỖI: Thiếu BABYLON_MNEMONIC trong biến môi trường.');
        process.exit(1);
    }
}

async function main() {
    validateRuntimeConfig();

    logger.info('🤖 ENGRAM TO BABYLON BATCHER');
    logger.info(`👉 Batch Size: ${config.BATCH_SIZE} | RPC: ${config.BABYLON_RPC}`);
    logger.info('⏳ Đang lắng nghe block mới từ Strata...');
    logger.info(`🗂️ Epoch-Batch Store: ${getEpochFilePath()}`);
    logger.info(`🗂️ Babylon Anchor Store: ${getBabylonAnchorFilePath()}`);
    logger.info(`🗂️ Babylon Pending Store: ${getBabylonPendingFilePath()}`);

    let lastProcessedHeight = 0;
    let batchBuffer = [];
    let currentEpoch = getNextEpoch();
    let pendingRecordedForEpoch = false;

    while (true) {
        const block = await getStrataBlock();

        if (block && block.height > lastProcessedHeight) {
            const exists = batchBuffer.find((item) => item.height === block.height);

            if (!exists) {
                batchBuffer.push(block);
                const indexInEpoch = batchBuffer.length;
                logger.info(`📥 Gom Block ${block.height} hash ${block.hash.substring(0, 10)}... (index: ${indexInEpoch}/${config.BATCH_SIZE} | epoch: ${currentEpoch})`);

                const stored = persistHeightEpoch(currentEpoch, block, indexInEpoch, config.BATCH_SIZE);
                if (stored) {
                    logger.info(`🧾 Đã lưu mapping epoch ${currentEpoch} -> batch (height ${block.height}, index ${indexInEpoch}/${config.BATCH_SIZE})`);
                }

                lastProcessedHeight = block.height;

                if (batchBuffer.length >= config.BATCH_SIZE) {
                    if (!pendingRecordedForEpoch) {
                        const pendingStored = persistBabylonPending(currentEpoch, batchBuffer);
                        if (pendingStored) {
                            logger.info(`🕒 Đã lưu pending submit cho epoch ${currentEpoch} (${batchBuffer.length} heights)`);
                            pendingRecordedForEpoch = true;
                        }
                    }

                    const anchorResult = await anchorBatch(batchBuffer);
                    if (anchorResult) {
                        const anchorStored = persistBabylonAnchor(currentEpoch, anchorResult);
                        if (anchorStored) {
                            logger.info(`🧾 Đã lưu anchor epoch ${currentEpoch} | tx ${anchorResult.txHash} | root ${anchorResult.merkleRoot.substring(0, 12)}...`);
                        }

                        logger.info(`✅ Hoàn tất epoch ${currentEpoch} (${batchBuffer.length}/${config.BATCH_SIZE})`);
                        currentEpoch += 1;
                        batchBuffer = [];
                        pendingRecordedForEpoch = false;
                    } else {
                        logger.info('⚠️ Gửi thất bại, sẽ thử lại ở lượt sau...');
                    }
                }
            }
        }

        await new Promise((resolve) => setTimeout(resolve, config.POLL_INTERVAL_MS));
    }
}

module.exports = {
    main,
};
