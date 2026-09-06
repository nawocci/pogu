export type Route =
  | { name: 'connections' }
  | { name: 'providers' }
  | { name: 'provider-detail'; providerId: number }
  | { name: 'groups' }
  | { name: 'group-detail'; groupId: number }
  | { name: 'monitoring' }
  | { name: 'configurations' }
  | { name: 'login' }
  | { name: 'not-found'; path: string };

export interface RouterState {
  path: string;
  search: string;
  route: Route;
}

export function parseRoute(pathname: string): Route {
  let p = pathname.trim();
  if (p.length > 1 && p.endsWith('/')) {
    p = p.replace(/\/+$/, '');
  }

  if (p === '' || p === '/' || p === '/connections') {
    return { name: 'connections' };
  }

  // pre-rename path kept as an alias
  if (p === '/api-keys') {
    return { name: 'connections' };
  }

  if (p === '/login') {
    return { name: 'login' };
  }

  if (p === '/providers') {
    return { name: 'providers' };
  }

  const match = p.match(/^\/providers\/(\d+)$/);
  if (match) {
    const providerId = parseInt(match[1], 10);
    if (providerId > 0) {
      return { name: 'provider-detail', providerId };
    }
  }

  if (p === '/groups') {
    return { name: 'groups' };
  }

  const groupMatch = p.match(/^\/groups\/(\d+)$/);
  if (groupMatch) {
    const groupId = parseInt(groupMatch[1], 10);
    if (groupId > 0) {
      return { name: 'group-detail', groupId };
    }
  }

  if (p === '/monitoring') {
    return { name: 'monitoring' };
  }

  if (p === '/configurations') {
    return { name: 'configurations' };
  }

  return { name: 'not-found', path: pathname };
}

export function getSafeRedirect(search: string, fallback = '/'): string {
  const params = new URLSearchParams(search);
  const redirect = params.get('redirect');
  if (!redirect) return fallback;
  if (redirect.startsWith('/') && !redirect.startsWith('//') && !redirect.startsWith('/login')) {
    return redirect;
  }
  return fallback;
}
