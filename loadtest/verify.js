import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';
import http from 'k6/http';
import { durationSeconds, writeSummary } from './lib.js';

const boardURL = __ENV.BOARD_URL;
const accessToken = __ENV.ACCESS_TOKEN;
const rate = Number(__ENV.RATE || 1000);
const duration = __ENV.DURATION || '20s';
const createOnly = __ENV.CREATE_ONLY === 'true';
const maxP99 = __ENV.MAX_P99 || '50';
const verifyDuration = new Trend('verify_duration', true);
const verifyErrors = new Rate('verify_errors');

if (!boardURL || !accessToken) throw new Error('BOARD_URL and ACCESS_TOKEN are required');

const thresholds = {
  verify_errors: ['rate==0'],
  iterations: [`count>=${Math.floor(rate * durationSeconds(duration) * 0.99)}`],
};
if (maxP99 !== 'none') thresholds.verify_duration = [`p(99)<${maxP99}`];

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  scenarios: {
    verify: {
      executor: 'constant-arrival-rate', rate, timeUnit: '1s', duration,
      preAllocatedVUs: Number(__ENV.PREALLOCATED_VUS || 200),
      maxVUs: Number(__ENV.MAX_VUS || 1000),
    },
  },
  thresholds,
};

export default function () {
  let response;
  if (createOnly || __ITER % 2 === 0) {
    response = http.post(`${boardURL}/board/v1/posts`, JSON.stringify({
      title: `scale-${__VU}-${__ITER}`, content: 'real authenticated Board write',
    }), {
      headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' },
    });
  } else {
    response = http.get(`${boardURL}/board/v1/posts?page_size=20`);
  }
	verifyDuration.add(response.timings.duration);
	verifyErrors.add(!check(response, { 'real Board operation succeeds': (r) => r.status === 200 }));
}

export function handleSummary(data) { return writeSummary(data); }
