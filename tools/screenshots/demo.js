// Shows a made-up "Sample Show" playing in the settings window, for the README screenshots (docs/images). Paste it
// into the page of a throwaway copy (see "Screenshots" in docs/development.md); nothing is sent to Discord or a player. It only
// changes what the page is told: the status pills, "Now playing", the "How it looks on Discord" card and Your stats.
(() => {
  const real = window.__realFetch || window.fetch;
  window.__realFetch = real;
  const start = Date.now() - 744000; // 12:24 into a 48-minute episode
  const status = {
    running: true, discord: 'connected', mpc: true, player: 'MPC-HC', nowPlaying: 'Sample Show - S01E02 - Episode title',
    paused: false, pauseCleared: false, hidden: false, lastError: null, since: start,
    preview: {
      watching: true, name: 'Sample Show', details: 'S01E02 · Episode title', state: 'Drama, Mystery · ★ 8.1',
      largeText: 'Season 1, Episode 2', start, end: start + 2880000, button: 'View on IMDb',
    },
  };
  const stats = { since: '2026-10-01', videos: 14, matched: 11, watchedMs: 19260000, players: { 'MPC-HC': 9, Plex: 4, VLC: 1 } };
  const answer = (v) => Promise.resolve(new Response(JSON.stringify(v), { headers: { 'Content-Type': 'application/json' } }));
  window.fetch = (url, opts) => {
    const u = String(url);
    if (u.endsWith('/api/status')) return answer(status);
    if (u.endsWith('/api/stats')) return answer(stats);
    return real(url, opts);
  };
})();
