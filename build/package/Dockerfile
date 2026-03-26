FROM mcr.microsoft.com/playwright:v1.58.2-noble

ARG VERSION=dev
ARG BUILD_RFC3339=unknown
ARG REVISION=unknown

ENV MCD_BUILD_VERSION=${VERSION}
ENV MCD_BUILD_RFC3339=${BUILD_RFC3339}
ENV MCD_BUILD_REVISION=${REVISION}

WORKDIR /app/apps/headless

COPY apps/headless/package.json ./
RUN npm install --omit=dev

COPY apps/headless/run.js ./run.js

WORKDIR /app
COPY packages/core ./packages/core

ENTRYPOINT ["node", "apps/headless/run.js"]
