// Free-tier baseline — 1 VU, ~4 upstream calls/min to stay under typical Gemini free RPM.
//
// Measures latency on successful completions only; some 502/429 from Google is expected.
// Do not use http_req_failed or overall checks for resume claims — use:
//   http_req_duration{expected_response:true}  →  p(95) / median
//
// Run:
//   export GATEWAY_API_KEY='your-plaintext-key'
//   make k6-baseline-free
import http from 'k6/http';
import { check, sleep } from 'k6';
import { jsonAuthHeaders } from './lib/auth.js';

const ITERATIONS = Number(__ENV.ITERATIONS || 10);
const PACE_SEC = Number(__ENV.PACE_SEC || 15);

export const options = {
  scenarios: {
    free_tier: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: ITERATIONS,
      maxDuration: '10m',
    },
  },
  thresholds: {
    // Only assert latency when we get at least one 200.
    'http_req_duration{expected_response:true}': ['p(95)<5000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const MODEL = __ENV.MODEL || 'gemini-2.5-flash';

export default function () {
  const payload = JSON.stringify({
    model: MODEL,
    messages: [{ role: 'user', content: 'Say hello in one word.' }],
  });

  const res = http.post(`${BASE_URL}/v1/chat/completions`, payload, {
    headers: jsonAuthHeaders(),
    tags: { name: 'chat_completion' },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
    'not unauthorized': (r) => r.status !== 401,
  });

  sleep(PACE_SEC);
}

export function handleSummary(data) {
  const ok200 = data.metrics['http_req_duration{expected_response:true}'];
  const checks = data.root_group?.checks || [];

  const statusCheck = checks.find((c) => c.name === 'status is 200');
  const successes = statusCheck?.passes ?? 0;
  const total = (statusCheck?.passes ?? 0) + (statusCheck?.fails ?? 0);

  console.log('\n--- Free-tier baseline summary (use for resume) ---');
  if (ok200?.values) {
    const v = ok200.values;
    console.log(`Successful completions: ${successes}/${total}`);
    console.log(`End-to-end p95 (200 only): ${v['p(95)']?.toFixed(0)}ms`);
    console.log(`End-to-end median (200 only): ${v.med?.toFixed(0)}ms`);
  } else {
    console.log('No HTTP 200 responses — check GEMINI_API_KEY and free-tier quota.');
  }
  console.log('(Ignore http_req_failed — free tier rejects most sustained load.)\n');

  return {};
}
