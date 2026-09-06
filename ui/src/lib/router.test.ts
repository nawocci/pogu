import test from 'node:test';
import assert from 'node:assert/strict';
import { parseRoute, getSafeRedirect } from './routes.ts';

test('Routing Model: root and /connections route to connections', () => {
  assert.deepEqual(parseRoute('/'), { name: 'connections' });
  assert.deepEqual(parseRoute('/connections'), { name: 'connections' });
  assert.deepEqual(parseRoute('/connections/'), { name: 'connections' });
  // pre-rename alias
  assert.deepEqual(parseRoute('/api-keys'), { name: 'connections' });
});

test('Routing Model: provider detail routes to provider-detail with parsed ID', () => {
  assert.deepEqual(parseRoute('/providers/1'), { name: 'provider-detail', providerId: 1 });
  assert.deepEqual(parseRoute('/providers/123'), { name: 'provider-detail', providerId: 123 });
  assert.deepEqual(parseRoute('/providers/999999'), { name: 'provider-detail', providerId: 999999 });
});

test('Routing Model: /login routes to login', () => {
  assert.deepEqual(parseRoute('/login'), { name: 'login' });
  assert.deepEqual(parseRoute('/login/'), { name: 'login' });
});

test('Routing Model: /groups and group detail routes', () => {
  assert.deepEqual(parseRoute('/groups'), { name: 'groups' });
  assert.deepEqual(parseRoute('/groups/'), { name: 'groups' });
  assert.deepEqual(parseRoute('/groups/1'), { name: 'group-detail', groupId: 1 });
  assert.deepEqual(parseRoute('/groups/123'), { name: 'group-detail', groupId: 123 });
  assert.deepEqual(parseRoute('/groups/0'), { name: 'not-found', path: '/groups/0' });
  assert.deepEqual(parseRoute('/groups/abc'), { name: 'not-found', path: '/groups/abc' });
});

test('Routing Model: /monitoring routes to monitoring', () => {
  assert.deepEqual(parseRoute('/monitoring'), { name: 'monitoring' });
  assert.deepEqual(parseRoute('/monitoring/'), { name: 'monitoring' });
});

test('Routing Constraints: no global /models route', () => {
  assert.deepEqual(parseRoute('/models'), { name: 'not-found', path: '/models' });
  assert.deepEqual(parseRoute('/models/123'), { name: 'not-found', path: '/models/123' });
});

test('Unknown routes map to not-found', () => {
  assert.deepEqual(parseRoute('/non-existent'), { name: 'not-found', path: '/non-existent' });
  assert.deepEqual(parseRoute('/providers/invalid-id'), { name: 'not-found', path: '/providers/invalid-id' });
  assert.deepEqual(parseRoute('/providers/0'), { name: 'not-found', path: '/providers/0' });
  assert.deepEqual(parseRoute('/providers/-1'), { name: 'not-found', path: '/providers/-1' });
});

test('Authentication: redirect target extraction', () => {
  assert.equal(getSafeRedirect('?redirect=%2Fproviders%2F123'), '/providers/123');
  assert.equal(getSafeRedirect('?redirect=%2Fconnections'), '/connections');
  assert.equal(getSafeRedirect('?redirect=%2Fproviders'), '/providers');
});

test('Authentication: redirect prevents loops and external open redirects', () => {
  assert.equal(getSafeRedirect('?redirect=%2Flogin'), '/');
  assert.equal(getSafeRedirect('?redirect=https%3A%2F%2Fmalicious.site'), '/');
  assert.equal(getSafeRedirect('?redirect=%2F%2Fmalicious.site'), '/');
  assert.equal(getSafeRedirect(''), '/');
});
