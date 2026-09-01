import { check } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import http from 'k6/http';
import { IDENTITY_URL, postJSON, signup, writeSummary } from './lib.js';

const callers = Number(__ENV.CALLERS || 50);
const successes = new Counter('rotation_successes');
const unexpected = new Rate('unexpected_responses');

export const options = {
  scenarios: { flood: { executor: 'shared-iterations', vus: callers, iterations: callers, maxDuration: '30s' } },
  thresholds: { rotation_successes: ['count==1'], unexpected_responses: ['rate==0'] },
};

export function setup() { return signup(Date.now() % 100000000).refreshToken; }

export default function (refreshToken) {
  const response = postJSON(`${IDENTITY_URL}/oauth2/v1/token`, {
    grant_type: 'refresh_token', refresh_token: refreshToken,
  }, { responseCallback: http.expectedStatuses(200, 401) });
  if (response.status === 200) successes.add(1);
  unexpected.add(!check(response, { 'rotation or reuse rejection, never 5xx': (r) => r.status === 200 || r.status === 401 }));
}

export function teardown(refreshToken) {
  const response = postJSON(`${IDENTITY_URL}/oauth2/v1/token`, {
    grant_type: 'refresh_token', refresh_token: refreshToken,
  }, { responseCallback: http.expectedStatuses(401) });
  if (response.status !== 401) throw new Error(`family survived reuse flood: ${response.status}`);
}

export function handleSummary(data) { return writeSummary(data); }
