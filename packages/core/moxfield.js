'use strict';

function buildCollectionUrlFromId(collectionId) {
  return `https://moxfield.com/collection/${collectionId}`;
}

function isValidCollectionUrl(urlString) {
  try {
    const u = new URL(urlString);
    const isHttps = u.protocol === 'https:';
    const isTargetHost = u.hostname === 'moxfield.com' || u.hostname === 'www.moxfield.com';
    const isCollectionPath = /^\/collection\/[A-Za-z0-9_-]+\/?$/.test(u.pathname);
    return isHttps && isTargetHost && isCollectionPath;
  } catch {
    return false;
  }
}

function extractCollectionIdFromUrl(urlString) {
  try {
    const u = new URL(urlString);
    const match = u.pathname.match(/^\/collection\/([A-Za-z0-9_-]+)\/?$/);
    return match ? match[1] : null;
  } catch {
    return null;
  }
}

module.exports = {
  buildCollectionUrlFromId,
  isValidCollectionUrl,
  extractCollectionIdFromUrl
};
