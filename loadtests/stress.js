import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '30s', target: 20 },   // ramp up
        { duration: '1m', target: 50 },   // sustained load
        { duration: '30s', target: 100 },  // push limits
        { duration: '30s', target: 0 },    // ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<500'],
        http_req_failed: ['rate<0.1'],
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
    const payload = JSON.stringify({
        model: 'gpt-4o-mini',
        messages: [{ role: 'user', content: 'Say hello in one word.' }],
    });

    const params = { headers: { 'Content-Type': 'application/json' } };
    const res = http.post(`${BASE_URL}/v1/chat/completions`, payload, params);

    check(res, {
        'status is 200 or 429': (r) => r.status === 200 || r.status === 429,
    });

    sleep(0.5);
}
