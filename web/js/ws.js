// WebSocket client for live listing availability (GET /ws/listings/{id},
// internal/realtime/websocket.go). One of the three real-time transport
// styles this project demonstrates — see sse.js (dashboard feed) and
// polling.js (fallback for clients that can't do WS/SSE).
(function () {
  function wsBase() {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host.includes("8081") || window.location.protocol === "https:"
      ? window.location.host // through Nginx, which proxies /ws/* (deployments/nginx/nginx.conf)
      : "localhost:8080";
    return `${proto}//${host}`;
  }

  function watchListingAvailability(listingId, handlers) {
    handlers = handlers || {};
    let socket;
    try {
      socket = new WebSocket(`${wsBase()}/ws/listings/${listingId}`);
    } catch (err) {
      handlers.onError && handlers.onError(err);
      return null;
    }

    socket.addEventListener("open", () => handlers.onOpen && handlers.onOpen());
    socket.addEventListener("message", (evt) => handlers.onMessage && handlers.onMessage(evt.data));
    socket.addEventListener("close", () => handlers.onClose && handlers.onClose());
    socket.addEventListener("error", (err) => handlers.onError && handlers.onError(err));
    return socket;
  }

  window.GearShareWS = { watchListingAvailability };
})();
