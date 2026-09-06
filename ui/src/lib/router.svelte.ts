import {
  type Route,
  type RouterState,
  parseRoute,
  getSafeRedirect,
} from './routes';

export {
  type Route,
  type RouterState,
  parseRoute,
  getSafeRedirect,
};

export const router = $state<RouterState>({
  path: typeof window !== 'undefined' ? window.location.pathname : '/',
  search: typeof window !== 'undefined' ? window.location.search : '',
  route: parseRoute(typeof window !== 'undefined' ? window.location.pathname : '/'),
});

function syncRoute(): void {
  if (typeof window === 'undefined') return;
  router.path = window.location.pathname;
  router.search = window.location.search;
  router.route = parseRoute(window.location.pathname);
}

export function navigate(to: string, options: { replace?: boolean } = {}): void {
  if (typeof window === 'undefined') return;

  const url = new URL(to, window.location.origin);
  const target = url.pathname + url.search + url.hash;

  if (options.replace) {
    window.history.replaceState(null, '', target);
  } else {
    window.history.pushState(null, '', target);
  }

  syncRoute();
}

function handleLinkClick(event: MouseEvent): void {
  if (event.defaultPrevented || event.button !== 0) return;
  if (event.metaKey || event.altKey || event.ctrlKey || event.shiftKey) return;

  const target = (event.target as Element | null)?.closest('a');
  if (!target || !(target instanceof HTMLAnchorElement)) return;

  if (target.target && target.target !== '_self') return;
  if (target.hasAttribute('download')) return;
  if (target.getAttribute('rel')?.includes('external')) return;

  const href = target.getAttribute('href');
  if (!href || href.startsWith('#') || href.startsWith('mailto:') || href.startsWith('tel:')) return;

  const url = new URL(target.href, window.location.href);
  if (url.origin !== window.location.origin) return;

  // Do not intercept backend/API endpoints
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/v1/') || url.pathname === '/healthz') {
    return;
  }

  event.preventDefault();
  navigate(url.pathname + url.search + url.hash);
}

export function initRouter(): () => void {
  if (typeof window === 'undefined') return () => {};

  syncRoute();
  window.addEventListener('popstate', syncRoute);
  window.addEventListener('click', handleLinkClick);

  return () => {
    window.removeEventListener('popstate', syncRoute);
    window.removeEventListener('click', handleLinkClick);
  };
}
