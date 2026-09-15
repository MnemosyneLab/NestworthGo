const encoder = new TextEncoder();

function bearerToken(request: Request): string | null {
  const header = request.headers.get("Authorization");
  if (!header) return null;

  const match = /^Bearer\s+(.+)$/i.exec(header);
  const token = match?.[1]?.trim();
  return token ? token : null;
}

async function digest(value: string): Promise<Uint8Array> {
  const bytes = await crypto.subtle.digest("SHA-256", encoder.encode(value));
  return new Uint8Array(bytes);
}

function constantTimeEqual(left: Uint8Array, right: Uint8Array): boolean {
  if (left.length !== right.length) return false;

  let difference = 0;
  for (let index = 0; index < left.length; index += 1) {
    difference |= left[index] ^ right[index];
  }
  return difference === 0;
}

/**
 * Checks a Bearer token without ever returning the configured secret or
 * including it in a cache key. Hashing both values gives the comparison a
 * fixed length before the constant-time byte comparison.
 */
export async function isAuthorized(request: Request, configuredToken?: string): Promise<boolean> {
  const suppliedToken = bearerToken(request);
  const expectedToken = configuredToken?.trim();
  if (!suppliedToken || !expectedToken) return false;

  const [suppliedDigest, expectedDigest] = await Promise.all([
    digest(suppliedToken),
    digest(expectedToken),
  ]);
  return constantTimeEqual(suppliedDigest, expectedDigest);
}
