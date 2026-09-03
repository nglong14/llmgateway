// Gateway auth expects the plaintext API key in Authorization: Bearer <key>.
// GATEWAY_API_KEY_HASH in .env is only for gateway config — do not pass the hash here.
export function gatewayApiKey() {
  const key = __ENV.GATEWAY_API_KEY;
  if (!key) {
    throw new Error(
      'GATEWAY_API_KEY is required (plaintext key that matches GATEWAY_API_KEY_HASH in .env)',
    );
  }
  return key;
}

export function authHeaders(extra = {}) {
  return {
    Authorization: `Bearer ${gatewayApiKey()}`,
    ...extra,
  };
}

export function jsonAuthHeaders() {
  return authHeaders({ 'Content-Type': 'application/json' });
}
