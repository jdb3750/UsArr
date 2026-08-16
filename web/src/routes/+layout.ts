// SPA, not a prerendered site: one fallback document (index.html) is emitted
// and every route resolves in the browser. ADR-0003 / ADR-0024 §6.
export const ssr = false;
export const prerender = false;
export const trailingSlash = 'never';
