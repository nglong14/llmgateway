import http from 'k6/http';
import { check, sleep } from 'k6';
import { authHeaders } from './lib/auth.js';

export const options = {
  vus: 5,
  duration: '30s',
  blacklistIPs: [],
  thresholds: {
    http_req_duration: ['p(95)<2000'],
    checks: ['rate>0.95'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  const health = http.get(`${BASE_URL}/health`);
  check(health, { 'health ok': (r) => r.status === 200 || r.status === 429 });

  const models = http.get(`${BASE_URL}/v1/models`, { headers: authHeaders() });
  check(models, {
    'models ok': (r) => r.status === 200 || r.status === 429,
    'models not unauthorized': (r) => r.status !== 401,
  });

  sleep(1);
}
