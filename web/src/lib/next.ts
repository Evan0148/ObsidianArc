/**
 * A destination carried through the sign-in page, kept to this site.
 *
 * The value ends up in a navigation, so anything that could name another host
 * — an absolute URL, a protocol-relative "//elsewhere", a backslash some
 * browsers read as a slash — is dropped rather than repaired. The server
 * applies the same rule to the one it builds; this is the half that runs
 * where the value is actually used.
 *
 * Here rather than beside the router, for the reason formatUptime is here:
 * the sign-in card needs this one function, and importing it from the router
 * would pull every screen in the routing table into the card's own graph.
 */
export function safeNext(raw: unknown): string {
  if (typeof raw !== 'string') return '';
  const value = raw.trim();
  if (!value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) return '';
  return value;
}
