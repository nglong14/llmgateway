import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const rejected = new Counter('rate_limited_requests');

export const options = {
    vus: 1,
    iterations: 50,
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
    const res = http.get(`${BASE_URL}/health`);

    if (res.status === 429) {
        rejected.add(1);
    }

    check(res, {
        'got response': (r) => r.status === 200 || r.status === 429,
    });

    // No sleep — intentionally rapid-fire to trigger rate limiter
}
