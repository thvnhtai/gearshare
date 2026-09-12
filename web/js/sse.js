// Server-Sent Events client for the owner dashboard's live booking-status
// feed (GET /api/v1/bookings/events, internal/realtime/sse.go). Chosen over
// WebSockets here because this feed is one-directional (server -> browser)
// and EventSource gives us automatic reconnection for free.
(function () {
  function watchBookingEvents(handlers) {
    handlers = handlers || {};
    const token = window.GearShare.getToken();
    if (!token) {
      handlers.onError && handlers.onError(new Error("not authenticated"));
      return null;
    }

    // EventSource can't set custom headers, so the access token travels as
    // a query param on this one endpoint only — see internal/realtime/sse.go
    // for the matching server-side fallback-to-query-param auth check.
    const url = `${window.GearShare.apiBase}/bookings/events?access_token=${encodeURIComponent(token)}`;
    const source = new EventSource(url);

    source.addEventListener("open", () => handlers.onOpen && handlers.onOpen());
    source.addEventListener("booking-update", (evt) => {
      try {
        handlers.onUpdate && handlers.onUpdate(JSON.parse(evt.data));
      } catch (err) {
        console.error("sse: bad payload", err);
      }
    });
    source.addEventListener("error", (err) => handlers.onError && handlers.onError(err));
    return source;
  }

  window.GearShareSSE = { watchBookingEvents };
})();
