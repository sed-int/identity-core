import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';
import { IDENTITY_URL, durationSeconds, postJSON, signup, writeSummary } from './lib.js';

const rate = Number(__ENV.RATE || 500);
const duration = __ENV.DURATION || '5m';
const maxVUs = Number(__ENV.MAX_VUS || 200);
const refreshDuration = new Trend('refresh_duration', true);
const refreshErrors = new Rate('refresh_errors');
let refreshToken;

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  setupTimeout: '10m',
  scenarios: {
    refresh: {
      executor: 'constant-arrival-rate', rate, timeUnit: '1s', duration,
      preAllocatedVUs: Math.min(100, maxVUs), maxVUs,
    },
  },
  thresholds: {
    refresh_duration: ['p(99)<100'],
    refresh_errors: ['rate<0.001'],
    iterations: [`count>=${Math.floor(rate * durationSeconds(duration) * 0.99)}`],
  },
};

export function setup() {
  const run = Date.now() % 100000;
  const tokens = [];
  for (let i = 0; i < maxVUs; i += 1) tokens.push(signup((run * 1000 + i) % 100000000).refreshToken);
  return tokens;
}

export default function (tokens) {
  if (!refreshToken) refreshToken = tokens[(__VU - 1) % tokens.length];
  const response = postJSON(`${IDENTITY_URL}/oauth2/v1/token`, {
    grant_type: 'refresh_token', refresh_token: refreshToken,
  });
  refreshDuration.add(response.timings.duration);
  const ok = check(response, { 'refresh returned rotated token': (r) => r.status === 200 && r.json('refreshToken') });
  refreshErrors.add(!ok);
  if (ok) refreshToken = response.json('refreshToken');
}

export function handleSummary(data) { return writeSummary(data); }
