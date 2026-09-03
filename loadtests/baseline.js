// Baseline latency benchmark — stays under the default 10 RPS client rate limit.
// Target: mostly HTTP 200 responses for resume-ready p95/p99 numbers.
//
// Run:
//   export GATEWAY_API_KEY='your-plaintext-key'
//   make k6-baseline
//
// Read k6 output:
//   http_req_duration{expected_response:true}  → end-to-end latency for 200s
// Compare with Grafana "Request Latency" panel during the same window.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { jsonAuthHeaders } from './lib/auth.js';

export const options = {
  vus: 5,
  duration: '3m',
  thresholds: {
    checks: ['rate>0.95'],
    'http_req_duration{expected_response:true}': ['p(95)<3000', 'p(99)<5000'],
    http_req_failed: ['rate<0.05'],
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
    'not rate limited': (r) => r.status !== 429,
  });

  sleep(1);
}
