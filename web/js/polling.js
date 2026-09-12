// Long/short-polling fallback (GET /api/v1/bookings/{id}/poll?since=<ts>,
// internal/realtime/polling.go) for clients that can't hold a WebSocket or
// EventSource open — e.g. some in-app mobile webviews, or a corporate proxy
// that kills long-lived connections. Used by dashboard.html only when SSE
// setup throws.
(function () {
  function pollBooking(bookingId, { intervalMs = 4000, onUpdate, onError } = {}) {
    let since = 0;
    let stopped = false;

    async function tick() {
      if (stopped) return;
      try {
        const res = await fetch(`${window.GearShare.apiBase}/bookings/${bookingId}/poll?since=${since}`, {
          headers: { Authorization: "Bearer " + window.GearShare.getToken() },
        });
        if (res.ok) {
          const body = await res.json();
          since = body.server_time;
          if (body.changed) onUpdate && onUpdate(body.booking);
        }
      } catch (err) {
        onError && onError(err);
      } finally {
        if (!stopped) setTimeout(tick, intervalMs);
      }
    }

    tick();
    return { stop: () => (stopped = true) };
  }

  window.GearSharePolling = { pollBooking };
})();
