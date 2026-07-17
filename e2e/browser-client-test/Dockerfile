FROM node:24-alpine

WORKDIR /app

COPY e2e/browser-client-test/package.json e2e/browser-client-test/package-lock.json ./
RUN npm ci --no-fund --no-audit

COPY e2e/browser-client-test/run.mjs ./run.mjs

CMD ["node", "run.mjs"]
