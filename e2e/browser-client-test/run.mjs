import { JSDOM } from 'jsdom';
import { createAnalytics } from '@tryformation/formation-web-analytics-client';

const endpoint = mustEnv('ANALYTICS_ENDPOINT');
const siteId = mustEnv('ANALYTICS_SITE_ID');
const elasticsearchURL = mustEnv('ELASTICSEARCH_URL');
const dataStream = mustEnv('ELASTICSEARCH_DATA_STREAM');
const pageURL = mustEnv('TEST_PAGE_URL');
const pageReferrer = process.env.TEST_PAGE_REFERRER ?? 'http://referrer.test/';

const dom = new JSDOM('<!doctype html><html><head><title>Browser Client Smoke Test</title></head><body></body></html>', {
  url: pageURL,
  referrer: pageReferrer,
  pretendToBeVisual: true
});

installBrowserGlobals(dom.window);

let lastError = null;
const analytics = createAnalytics({
  endpoint,
  siteId,
  autoPageviews: false,
  sendBeacon: false,
  onError(error) {
    lastError = error;
  }
});

analytics.page({ smoke_test: true });

await wait(1000);

if (lastError) {
  throw new Error(`analytics client failed to deliver event: ${lastError.kind ?? 'unknown'} ${lastError.message}`);
}

const document = await waitForIndexedEvent(elasticsearchURL, dataStream, siteId);
if (document.site_id !== siteId) {
  throw new Error(`expected indexed site_id ${siteId}, got ${document.site_id}`);
}
const expectedIndexedURL = new URL(pageURL);
expectedIndexedURL.search = '';
expectedIndexedURL.hash = '';
if (document.url !== expectedIndexedURL.toString()) {
  throw new Error(`expected indexed url ${expectedIndexedURL.toString()}, got ${document.url}`);
}
if (document.request_domain !== 'collector') {
  throw new Error(`expected request_domain collector, got ${document.request_domain}`);
}
if (document.origin !== 'http://test-site') {
  throw new Error(`expected origin http://test-site, got ${document.origin}`);
}

console.log(`Browser client smoke test passed: indexed ${siteId} into ${dataStream}`);

function mustEnv(name) {
  const value = process.env[name];
  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

function installBrowserGlobals(window) {
  Object.defineProperty(globalThis, 'window', { value: window, configurable: true });
  Object.defineProperty(globalThis, 'document', { value: window.document, configurable: true });
  Object.defineProperty(globalThis, 'navigator', { value: window.navigator, configurable: true });
  Object.defineProperty(globalThis, 'location', { value: window.location, configurable: true });
  Object.defineProperty(globalThis, 'history', { value: window.history, configurable: true });
  Object.defineProperty(globalThis, 'localStorage', { value: window.localStorage, configurable: true });
  Object.defineProperty(globalThis, 'sessionStorage', { value: window.sessionStorage, configurable: true });
  Object.defineProperty(globalThis, 'crypto', { value: globalThis.crypto, configurable: true });

  const nativeFetch = globalThis.fetch.bind(globalThis);
  const browserFetch = (input, init = {}) => {
    const headers = new Headers(init.headers ?? {});
    if (!headers.has('Origin')) {
      headers.set('Origin', window.location.origin);
    }
    return nativeFetch(input, { ...init, headers });
  };

  Object.defineProperty(window, 'fetch', { value: browserFetch, configurable: true });
  Object.defineProperty(globalThis, 'fetch', { value: browserFetch, configurable: true });
}

async function waitForIndexedEvent(esURL, stream, expectedSiteID) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const response = await fetch(`${esURL}/${stream}/_search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        size: 1,
        query: {
          term: {
            site_id: expectedSiteID
          }
        }
      })
    });
    if (!response.ok) {
      throw new Error(`failed to query Elasticsearch: ${response.status}`);
    }
    const body = await response.json();
    const hit = body?.hits?.hits?.[0]?._source;
    if (hit) {
      return hit;
    }
    await wait(1000);
  }
  throw new Error(`timed out waiting for indexed event for ${expectedSiteID}`);
}

function wait(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
