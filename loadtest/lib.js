import http from 'k6/http';
import { check } from 'k6';

export const IDENTITY_URL = __ENV.IDENTITY_URL || 'http://host.docker.internal:8090';
export const BOARD_URL = __ENV.BOARD_URL || 'http://host.docker.internal:8091';
export const JSON_HEADERS = { 'Content-Type': 'application/json' };

export function postJSON(url, body, params = {}) {
  params.headers = Object.assign({}, JSON_HEADERS, params.headers || {});
  return http.post(url, JSON.stringify(body), params);
}

export function signup(suffix, device = '') {
  const phone = `+8216${String(suffix).padStart(8, '0').slice(-8)}`;
  const otp = postJSON(`${IDENTITY_URL}/auth/v1/otp/request`, { phone_number: phone }, { tags: { phase: 'seed' } });
  check(otp, { 'seed OTP requested': (r) => r.status === 200 });
  const code = otp.json('debugCode');
  const verified = postJSON(`${IDENTITY_URL}/auth/v1/otp/verify`, {
    phone_number: phone,
    code,
    device_fingerprint: device,
    device_name: 'phase7-k6',
  }, { tags: { phase: 'seed' } });
  const flow = verified.json('flowToken');
  const created = postJSON(`${IDENTITY_URL}/auth/v1/signup`, {
    flow_token: flow,
    nickname: 'phase7-k6',
    device_fingerprint: device,
    device_name: 'phase7-k6',
  }, { tags: { phase: 'seed' } });
  if (created.status !== 200) {
    throw new Error(`seed signup failed (${created.status}): ${created.body}`);
  }
  return created.json('tokens');
}

export function durationSeconds(raw) {
  const match = /^(\d+)(s|m)$/.exec(raw);
  if (!match) throw new Error(`duration must use whole seconds/minutes: ${raw}`);
  return Number(match[1]) * (match[2] === 'm' ? 60 : 1);
}

export function writeSummary(data) {
	const path = __ENV.SUMMARY_PATH || '/artifacts/summary.json';
	const metrics = data.metrics || {};
	const iterations = metrics.iterations ? metrics.iterations.values.count : 0;
	const dropped = metrics.dropped_iterations ? metrics.dropped_iterations.values.count : 0;
	return {
		[path]: JSON.stringify(data, null, 2),
		stdout: `iterations=${iterations} dropped=${dropped}\nsummary=${path}\n`,
	};
}
