import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';
import { IDENTITY_URL, durationSeconds, postJSON, writeSummary } from './lib.js';

const rate = Number(__ENV.RATE || 100);
const duration = __ENV.DURATION || '1m';
const users = Number(__ENV.LOGIN_USERS || 2000);
const maxVUs = Number(__ENV.MAX_VUS || 500);
const run = String(__ENV.LOGIN_RUN || '0001').padStart(4, '0');
const loginDuration = new Trend('login_duration', true);
const loginErrors = new Rate('login_errors');

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  scenarios: {
    login: {
      executor: 'constant-arrival-rate', rate, timeUnit: '1s', duration,
      preAllocatedVUs: Math.min(150, maxVUs), maxVUs,
    },
  },
  thresholds: {
    login_duration: ['p(99)<300'],
    login_errors: ['rate<0.001'],
    iterations: [`count>=${Math.floor(rate * durationSeconds(duration) * 0.99)}`],
  },
};

export default function () {
  const usersPerVU = Math.floor(users / maxVUs);
  if (usersPerVU < 1) throw new Error('LOGIN_USERS must be >= MAX_VUS');
  const index = (((__VU - 1) * usersPerVU + (__ITER % usersPerVU)) % users) + 1;
  const phone = `+82${run}${String(index).padStart(6, '0')}`;
  const otp = postJSON(`${IDENTITY_URL}/auth/v1/otp/request`, { phone_number: phone });
  if (otp.status !== 200) { loginErrors.add(true); return; }
  const response = postJSON(`${IDENTITY_URL}/auth/v1/otp/verify`, {
    phone_number: phone, code: otp.json('debugCode'),
  });
  loginDuration.add(response.timings.duration);
  loginErrors.add(!check(response, {
    'active login issued tokens': (r) => r.status === 200 && r.json('nextStep') === 'NEXT_STEP_TOKENS_ISSUED' && r.json('tokens.accessToken'),
  }));
}

export function handleSummary(data) { return writeSummary(data); }
