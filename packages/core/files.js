'use strict';

const crypto = require('crypto');
const path = require('path');

function sanitizeApiPathSegment(name) {
  // Very conservative sanitize: remove directory separators and control chars.
  return String(name).replace(/[/\\?%*:|"<>]/g, '-').replace(/\s+/g, '-');
}

function computeApiResponseRelativePath(urlString, method) {
  try {
    const u = new URL(urlString);
    const parts = u.pathname.split('/').filter(Boolean).map(sanitizeApiPathSegment);

    if (parts.length === 0) parts.push('index');

    let base = parts.pop() || 'index';
    if ((method || 'GET').toUpperCase() === 'GET' && u.search && u.search !== '') {
      const hash = crypto.createHash('sha1').update(u.search).digest('hex').slice(0, 7);
      base = `${base}-${hash}`;
    }

    const filename = `${base}.json`;
    return path.join(...parts, filename);
  } catch {
    const hash = crypto.createHash('sha1').update(String(urlString)).digest('hex').slice(0, 7);
    return `response-${hash}.json`;
  }
}

function sanitizeOutputFileName(name) {
  return String(name).replace(/[^A-Za-z0-9_.-]/g, '-');
}

module.exports = {
  computeApiResponseRelativePath,
  sanitizeApiPathSegment,
  sanitizeOutputFileName
};
