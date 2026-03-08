import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 10,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  // Health check
  const health = http.get(`${BASE_URL}/health`);
  check(health, { 'health 200': (r) => r.status === 200 });

  // List models
  const models = http.get(`${BASE_URL}/v1/models`);
  check(models, { 'models 200': (r) => r.status === 200 });

  sleep(1);
}
