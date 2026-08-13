// Link helpers.

const LOOPBACK = new Set(["127.0.0.1", "localhost", "0.0.0.0", "::1", "[::1]"]);

/**
 * Makes a server-side configured URL openable from the user's browser: the
 * config points apps at the server's own loopback (http://127.0.0.1:7878),
 * which is meaningless anywhere else — swap the host for the one the panel
 * is being viewed from, keeping port and path. Non-loopback hosts (real
 * domains, reverse-proxied apps) pass through untouched.
 */
export function externalURL(configured: string): string {
  try {
    const url = new URL(configured);
    if (LOOPBACK.has(url.hostname)) {
      url.hostname = window.location.hostname;
    }
    return url.toString();
  } catch {
    return configured;
  }
}
