const fs = require('fs');
const path = require('path');

const logger = require('./logger');

const defaultEpochFilePath = path.join(__dirname, '../logs/epoch-batch-map.jsonl');
const defaultBabylonAnchorFilePath = path.join(__dirname, '../logs/babylon-anchor-receipts.jsonl');
const defaultBabylonPendingFilePath = path.join(__dirname, '../logs/babylon-pending-submits.jsonl');

function getEpochFilePath() {
    const configuredPath = process.env.EPOCH_BATCH_FILE || process.env.HEIGHT_EPOCH_FILE;

    if (!configuredPath) {
        return defaultEpochFilePath;
    }

    if (path.isAbsolute(configuredPath)) {
        return configuredPath;
    }

    return path.resolve(process.cwd(), configuredPath);
}

function getBabylonAnchorFilePath() {
    const configuredPath = process.env.BABYLON_ANCHOR_FILE;

    if (!configuredPath) {
        return defaultBabylonAnchorFilePath;
    }

    if (path.isAbsolute(configuredPath)) {
        return configuredPath;
    }

    return path.resolve(process.cwd(), configuredPath);
}

function getBabylonPendingFilePath() {
    const configuredPath = process.env.BABYLON_PENDING_FILE;

    if (!configuredPath) {
        return defaultBabylonPendingFilePath;
    }

    if (path.isAbsolute(configuredPath)) {
        return configuredPath;
    }

    return path.resolve(process.cwd(), configuredPath);
}

function ensureStoreDir(filePath) {
    const storeDir = path.dirname(filePath);
    fs.mkdirSync(storeDir, { recursive: true });
}

function toBatchId(epoch) {
    return `batch-epoch-${epoch}`;
}

function getNextEpoch() {
    const filePath = getEpochFilePath();

    ensureStoreDir(filePath);

    if (!fs.existsSync(filePath)) {
        return 1;
    }

    const content = fs.readFileSync(filePath, 'utf8').trim();
    if (!content) {
        return 1;
    }

    let maxEpoch = 0;
    const lines = content.split('\n');

    for (const line of lines) {
        if (!line.trim()) {
            continue;
        }

        try {
            const parsed = JSON.parse(line);
            if (typeof parsed.epoch === 'number' && parsed.epoch > maxEpoch) {
                maxEpoch = parsed.epoch;
            }
        } catch (error) {
            logger.warn(`⚠️ Bỏ qua dòng không hợp lệ trong height-epoch store: ${line}`);
        }
    }

    return maxEpoch + 1;
}

function persistBatchEpoch(epoch, batch) {
    const filePath = getEpochFilePath();
    ensureStoreDir(filePath);

    const savedAt = new Date().toISOString();
    const batchId = toBatchId(epoch);
    const lines = batch
        .map((block) => JSON.stringify({
            epoch,
            batchId,
            height: block.height,
            hash: block.hash,
            savedAt,
        }))
        .join('\n');

    try {
        fs.appendFileSync(filePath, `${lines}\n`);
        return true;
    } catch (error) {
        logger.error(`❌ Không thể ghi height-epoch store: ${error.message}`);
        return false;
    }
}

function persistHeightEpoch(epoch, block, indexInEpoch, epochCount) {
    const filePath = getEpochFilePath();
    ensureStoreDir(filePath);
    const batchId = toBatchId(epoch);

    const line = JSON.stringify({
        epoch,
        batchId,
        batch: {
            indexInEpoch,
            epochCount,
        },
        strata: {
            height: block.height,
            hash: block.hash,
        },
        mapping: 'epoch_to_batch',
        savedAt: new Date().toISOString(),
    });

    try {
        fs.appendFileSync(filePath, `${line}\n`);
        return true;
    } catch (error) {
        logger.error(`❌ Không thể ghi height-epoch store: ${error.message}`);
        return false;
    }
}

function persistBabylonAnchor(epoch, anchorResult) {
    const filePath = getBabylonAnchorFilePath();
    ensureStoreDir(filePath);
    const batchId = toBatchId(epoch);

    const line = JSON.stringify({
        epoch,
        batchId,
        state: 'anchored',
        txHash: anchorResult.txHash,
        merkleRoot: anchorResult.merkleRoot,
        leafCount: anchorResult.leafCount,
        payload: anchorResult.memoPayload,
        startHeight: anchorResult.startHeight,
        endHeight: anchorResult.endHeight,
        strataHeights: anchorResult.strataHeights,
        savedAt: new Date().toISOString(),
    });

    try {
        fs.appendFileSync(filePath, `${line}\n`);
        return true;
    } catch (error) {
        logger.error(`❌ Không thể ghi babylon anchor store: ${error.message}`);
        return false;
    }
}

function persistBabylonPending(epoch, batch) {
    const filePath = getBabylonPendingFilePath();
    ensureStoreDir(filePath);

    const strataHeights = batch.map((item) => item.height);
    const startHeight = strataHeights[0];
    const endHeight = strataHeights[strataHeights.length - 1];
    const batchId = toBatchId(epoch);

    const line = JSON.stringify({
        epoch,
        batchId,
        state: 'pending_submit',
        startHeight,
        endHeight,
        leafCount: batch.length,
        strataHeights,
        savedAt: new Date().toISOString(),
    });

    try {
        fs.appendFileSync(filePath, `${line}\n`);
        return true;
    } catch (error) {
        logger.error(`❌ Không thể ghi babylon pending store: ${error.message}`);
        return false;
    }
}

module.exports = {
    getEpochFilePath,
    getBabylonAnchorFilePath,
    getBabylonPendingFilePath,
    getNextEpoch,
    toBatchId,
    persistHeightEpoch,
    persistBatchEpoch,
    persistBabylonPending,
    persistBabylonAnchor,
};
