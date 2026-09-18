// The create surface is ADR-058 Exception #2: it opts out of the normal shell.
// The (app) layout reads this via page.route.id (see gate.isExceptionRoute).
// The canonical composer route is /studio/create; /create 308-redirects here.
export const ssr = true;
