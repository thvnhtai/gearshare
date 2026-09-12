// Shared header/nav rendering + route guarding, included on every page.
(function () {
  function renderNav() {
    const nav = document.getElementById("site-nav");
    if (!nav) return;

    const user = window.GearShare.getUser();
    if (user) {
      nav.innerHTML = `
        <a href="index.html">Browse</a>
        <a href="dashboard.html">Dashboard</a>
        <span class="muted">${user.display_name} (${user.role})</span>
        <a href="#" id="logout-link">Log out</a>
      `;
      document.getElementById("logout-link").addEventListener("click", (e) => {
        e.preventDefault();
        window.GearShare.clearSession();
        window.location.href = "index.html";
      });
    } else {
      nav.innerHTML = `
        <a href="index.html">Browse</a>
        <a href="login.html">Log in</a>
        <a href="register.html">Sign up</a>
      `;
    }
  }

  function requireAuth() {
    if (!window.GearShare.isAuthenticated()) {
      window.location.href = "login.html";
    }
  }

  window.GearShareAuth = { renderNav, requireAuth };
  document.addEventListener("DOMContentLoaded", renderNav);
})();
