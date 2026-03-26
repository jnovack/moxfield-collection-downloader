FROM mcr.microsoft.com/playwright:v1.58.2-noble

WORKDIR /app/apps/headless

COPY apps/headless/package.json ./
RUN npm install --omit=dev

COPY apps/headless/run.js ./run.js

WORKDIR /app
COPY packages/core ./packages/core

ENTRYPOINT ["node", "apps/headless/run.js"]
