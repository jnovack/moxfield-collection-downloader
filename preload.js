// preload.js
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('electronAPI', {
  sendApiResponse: (payload) => {
    // keep payload minimal and stringified body
    try {
      ipcRenderer.send('save-api-response', payload);
    } catch (e) {
      // ignore
    }
  }
});

// Inject an observer script into the page context that wraps fetch and XHR.
// We inject by adding a <script> element so the wrappers run in page context
// (so they observe fetch/XHR executed by the page and preserve cookies/CORS).
(function inject() {
  const scriptContent = `
(function() {
  // Helper: only target api2.moxfield.com
  function isTargetHost(url) {
    try {
      const u = new URL(url, location.href);
      return u.hostname === 'api2.moxfield.com' || u.hostname.endsWith('.api2.moxfield.com');
    } catch (e) { return false; }
  }

  // --- wrap fetch ---
  const _fetch = window.fetch;
  if (_fetch) {
    window.fetch = function(input, init) {
      // call original
      return _fetch.apply(this, arguments).then(function(response) {
        try {
          const url = response.url || (typeof input === 'string' ? input : (input && input.url));
          if (url && isTargetHost(url)) {
            // clone to read body safely
            try {
              const clone = response.clone();
              clone.text().then(function(text) {
                try {
                  const method = (init && init.method) || 'GET';
                  // send minimal payload to main
                  if (window.electronAPI && window.electronAPI.sendApiResponse) {
                    window.electronAPI.sendApiResponse({ url: url, method: method, body: text });
                  }
                } catch (e) {}
              }).catch(()=>{});
            } catch (e) {}
          }
        } catch (e) {}
        return response;
      });
    };
  }

  // --- wrap XMLHttpRequest ---
  (function() {
    const origOpen = XMLHttpRequest.prototype.open;
    const origSend = XMLHttpRequest.prototype.send;

    XMLHttpRequest.prototype.open = function(method, url) {
      try {
        this._methodForSave = method;
        this._urlForSave = url;
      } catch (e) {}
      return origOpen.apply(this, arguments);
    };

    XMLHttpRequest.prototype.send = function(body) {
      try {
        this.addEventListener('readystatechange', function() {
          try {
            if (this.readyState === 4) {
              const fullUrl = this._urlForSave || '';
              // Resolve relative URLs
              let resolvedUrl = fullUrl;
              try {
                resolvedUrl = new URL(fullUrl, location.href).href;
              } catch (e) {}
              if (isTargetHost(resolvedUrl)) {
                let responseText = null;
                try {
                  // responseType might be json or text etc; responseText is usually available
                  responseText = this.responseText;
                } catch (e) {
                  responseText = null;
                }
                if (window.electronAPI && window.electronAPI.sendApiResponse) {
                  window.electronAPI.sendApiResponse({
                    url: resolvedUrl,
                    method: this._methodForSave || 'GET',
                    body: (typeof responseText === 'string') ? responseText : ''
                  });
                }
              }
            }
          } catch (e) {}
        });
      } catch (e) {}
      return origSend.apply(this, arguments);
    };
  })();
})();
`;

  function doInject() {
    try {
      const script = document.createElement('script');
      script.textContent = scriptContent;
      script.setAttribute('data-injected-by', 'jnovack-moxfield-downloader');
      (document.head || document.documentElement).appendChild(script);
      script.remove();
    } catch (e) {
      console.error("Injection failed:", e);
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", doInject);
  } else {
    doInject();
  }

  const script = document.createElement('script');
  script.textContent = scriptContent;
  // Mark it so other code can see it's from preload
  script.setAttribute('data-injected-by', 'jnovack-moxfield-downloader');
  document.documentElement.appendChild(script);
  script.remove();
})();
