// Thin fetch wrapper shared by every page. No framework, no build step —
// this is loaded as a plain <script> and exposes `window.GearShare`.
(function () {
  const API_BASE = window.location.origin.includes("8081") || window.location.origin.includes("443")
    ? "/api/v1" // served through Nginx (deployments/nginx/nginx.conf proxies /api/* to the Go backend)
    : "http://localhost:8080/api/v1"; // direct-to-backend for `file://`/plain dev serving

  const TOKEN_KEY = "gearshare_access_token";
  const USER_KEY = "gearshare_user";

  function getToken() {
    return localStorage.getItem(TOKEN_KEY);
  }

  function setSession(accessToken, user) {
    localStorage.setItem(TOKEN_KEY, accessToken);
    localStorage.setItem(USER_KEY, JSON.stringify(user));
  }

  function clearSession() {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
  }

  function getUser() {
    const raw = localStorage.getItem(USER_KEY);
    return raw ? JSON.parse(raw) : null;
  }

  async function request(path, options) {
    options = options || {};
    const headers = Object.assign({ "Content-Type": "application/json" }, options.headers || {});
    const token = getToken();
    if (token) headers["Authorization"] = "Bearer " + token;

    const res = await fetch(API_BASE + path, Object.assign({}, options, { headers }));
    if (res.status === 304) return null; // ETag cache hit, caller should keep prior data
    if (!res.ok) {
      let message = "Request failed (" + res.status + ")";
      try {
        const body = await res.json();
        if (body && body.error) message = body.error;
      } catch (_) {
        /* non-JSON error body */
      }
      throw new Error(message);
    }
    if (res.status === 204) return null;
    return res.json();
  }

  window.GearShare = {
    apiBase: API_BASE,
    getToken,
    setSession,
    clearSession,
    getUser,
    isAuthenticated: () => !!getToken(),

    register: (email, password, displayName, role) =>
      request("/auth/register", { method: "POST", body: JSON.stringify({ email, password, display_name: displayName, role }) }),
    login: (email, password) =>
      request("/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),

    listCategories: () => request("/categories"),
    listListings: (limit, offset) => request(`/listings?limit=${limit || 20}&offset=${offset || 0}`),
    getListing: (id) => request(`/listings/${id}`),
    createListing: (payload) => request("/listings", { method: "POST", body: JSON.stringify(payload) }),

    listMyBookings: () => request("/bookings"),
    createBooking: (listingId, startDate, endDate) =>
      request("/bookings", { method: "POST", body: JSON.stringify({ listing_id: listingId, start_date: startDate, end_date: endDate }) }),
    getBooking: (id) => request(`/bookings/${id}`),
    approveBooking: (id) => request(`/bookings/${id}/approve`, { method: "POST" }),
    rejectBooking: (id) => request(`/bookings/${id}/reject`, { method: "POST" }),
    cancelBooking: (id) => request(`/bookings/${id}/cancel`, { method: "POST" }),
    completeBooking: (id) => request(`/bookings/${id}/complete`, { method: "POST" }),

    createReview: (bookingId, rating, comment) =>
      request("/reviews", { method: "POST", body: JSON.stringify({ booking_id: bookingId, rating, comment }) }),
  };
})();
