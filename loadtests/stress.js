import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    blacklistIPs: [],
    stages: [
        { duration: '30s', target: 20 },   // ramp up
        { duration: '1m', target: 50 },   // sustained load
        { duration: '30s', target: 100 },  // push limits
        { duration: '30s', target: 0 },    // ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<2000'],
        checks: ['rate>0.95'],
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
    const payload = JSON.stringify({
        model: 'gemini-2.5-flash',
        messages: [{ role: 'user', content: 'Say hello in one word.' }],
    });

    const params = { headers: { 'Content-Type': 'application/json' } };
    const res = http.post(`${BASE_URL}/v1/chat/completions`, payload, params);

    check(res, {
        'valid response': (r) => r.status === 200 || r.status === 429,
    });

    sleep(0.5);
}
