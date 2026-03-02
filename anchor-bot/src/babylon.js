const { DirectSecp256k1HdWallet } = require('@cosmjs/proto-signing');
const { SigningStargateClient, assertIsDeliverTxSuccess } = require('@cosmjs/stargate');

const config = require('./config');
const logger = require('./logger');

async function submitToBabylon(memoData) {
    try {
        const wallet = await DirectSecp256k1HdWallet.fromMnemonic(config.MNEMONIC, { prefix: 'bbn' });
        const [account] = await wallet.getAccounts();
        const client = await SigningStargateClient.connectWithSigner(config.BABYLON_RPC, wallet);

        const amount = { denom: config.BABYLON_DENOM, amount: '1' };
        const fee = {
            amount: [{ denom: config.BABYLON_DENOM, amount: '500' }],
            gas: '250000',
        };

        logger.info(`   🚀 Đang bắn lên Babylon từ ví: ${account.address}`);

        const result = await client.sendTokens(
            account.address,
            account.address,
            [amount],
            fee,
            memoData
        );

        assertIsDeliverTxSuccess(result);
        return result.transactionHash;
    } catch (error) {
        logger.error(`   ❌ Lỗi Babylon: ${error.message}`);
        return null;
    }
}

module.exports = {
    submitToBabylon,
};
