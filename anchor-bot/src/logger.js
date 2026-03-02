const fs = require('fs');
const path = require('path');
const winston = require('winston');

const logsDir = path.join(__dirname, '../logs');
fs.mkdirSync(logsDir, { recursive: true });

const logger = winston.createLogger({
    level: 'info',
    format: winston.format.combine(
        winston.format.timestamp({ format: 'YYYY-MM-DD HH:mm:ss' }),
        winston.format.printf(({ timestamp, level, message }) => `[${timestamp}] ${level.toUpperCase()}: ${message}`)
    ),
    transports: [
        new winston.transports.Console(),
        new winston.transports.File({
            filename: path.join(logsDir, 'bot.log'),
            maxsize: 5242880000,
            maxFiles: 5,
        }),
        new winston.transports.File({
            filename: path.join(logsDir, 'error.log'),
            level: 'error',
        }),
    ],
});

module.exports = logger;
