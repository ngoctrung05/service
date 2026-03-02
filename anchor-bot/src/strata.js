const axios = require('axios');
const config = require('./config');

async function getStrataBlock() {
    try {
        const response = await axios.get(`${config.STRATA_RPC}/status`);
        const info = response.data.result.sync_info;

        return {
            height: Number.parseInt(info.latest_block_height, 10),
            hash: info.latest_block_hash,
        };
    } catch (_) {
        return null;
    }
}

module.exports = {
    getStrataBlock,
};
