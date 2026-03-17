FROM node:22-alpine

WORKDIR /app

COPY e2e/browser-client-test/package.json ./package.json
RUN npm install --no-fund --no-audit

COPY e2e/browser-client-test/run.mjs ./run.mjs

CMD ["node", "run.mjs"]
