import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const distPath = resolve(import.meta.dirname, '../../../internal/web/dist');
const html = readFileSync(resolve(distPath, 'index.html'), 'utf-8');

test('SPA dist bundle verification', () => {
  assert.match(html, /<div id="app"/, 'HTML contains root app element');
  assert.match(html, /<script type="module" crossorigin src="\/assets\/index-.*\.js">/, 'HTML loads bundle JS');
  assert.match(html, /<link rel="stylesheet" crossorigin href="\/assets\/index-.*\.css">/, 'HTML loads bundle CSS');

  const jsMatch = html.match(/src="\/assets\/(index-[^"]+\.js)"/);
  assert.ok(jsMatch, 'JS bundle script match found');
  const jsContent = readFileSync(resolve(distPath, 'assets', jsMatch[1]), 'utf-8');

  // Verify compiled bundle includes routing logic and routes
  assert.ok(jsContent.includes('/providers'), 'Bundle contains /providers route');
  assert.ok(jsContent.includes('/groups'), 'Bundle contains /groups route');
  assert.ok(jsContent.includes('/monitoring'), 'Bundle contains /monitoring route');
  assert.ok(jsContent.includes('/configurations'), 'Bundle contains /configurations route');
  assert.ok(jsContent.includes('/connections'), 'Bundle contains /connections route');
  assert.ok(jsContent.includes('/login'), 'Bundle contains /login route');
  assert.ok(jsContent.includes('Page not found'), 'Bundle contains Page not found');
  assert.ok(jsContent.includes('popstate'), 'Bundle listens to popstate');
  assert.ok(jsContent.includes('pushState'), 'Bundle pushes state for navigation');
});
