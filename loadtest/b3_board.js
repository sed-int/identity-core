import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';
import http from 'k6/http';
import { BOARD_URL, durationSeconds, postJSON, signup, writeSummary } from './lib.js';

const rate = Number(__ENV.RATE || 1000);
const duration = __ENV.DURATION || '1m';
const boardDuration = new Trend('board_duration', true);
const boardErrors = new Rate('board_errors');

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  scenarios: {
    board: {
      executor: 'constant-arrival-rate', rate, timeUnit: '1s', duration,
      preAllocatedVUs: Number(__ENV.PREALLOCATED_VUS || 300), maxVUs: Number(__ENV.MAX_VUS || 1000),
    },
  },
  thresholds: {
    board_duration: ['p(99)<50'], board_errors: ['rate<0.001'],
    iterations: [`count>=${Math.floor(rate * durationSeconds(duration) * 0.99)}`],
  },
};

export function setup() { return signup(Date.now() % 100000000).accessToken; }

export default function (accessToken) {
  let response;
  if (__ITER % 2 === 0) {
    response = postJSON(`${BOARD_URL}/board/v1/posts`, { title: `b3-${__VU}-${__ITER}`, content: 'phase7 load' }, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
	} else {
		response = http.get(`${BOARD_URL}/board/v1/posts?page_size=20`);
  }
  boardDuration.add(response.timings.duration);
  boardErrors.add(!check(response, { 'board request succeeded': (r) => r.status === 200 }));
}

export function handleSummary(data) { return writeSummary(data); }
