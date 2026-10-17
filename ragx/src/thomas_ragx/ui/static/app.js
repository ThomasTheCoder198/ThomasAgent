// Runs in <head> so the saved theme applies before first paint; no inline script (CSP script-src 'self').
(function () {
  var KEY = "ragx-theme";
  var media = window.matchMedia("(prefers-color-scheme: dark)");

  function saved() {
    try { return localStorage.getItem(KEY) || "system"; } catch (error) { return "system"; }
  }

  function apply(choice) {
    var dark = choice === "dark" || (choice === "system" && media.matches);
    document.documentElement.setAttribute("data-theme", dark ? "dark" : "light");
    document.querySelectorAll("[data-theme-choice]").forEach(function (button) {
      button.setAttribute("aria-pressed", String(button.getAttribute("data-theme-choice") === choice));
    });
  }

  function setMenu(open) {
    var toggle = document.querySelector("[data-menu-toggle]");
    var scrim = document.querySelector("[data-menu-close]");
    if (!toggle || !scrim) { return; }
    document.documentElement.toggleAttribute("data-menu-open", open);
    toggle.setAttribute("aria-expanded", String(open));
    scrim.hidden = !open;
    if (open) {
      var first = document.querySelector("#sign-panel a, #sign-panel button");
      if (first) { first.focus(); }
    }
  }

  apply(saved());
  media.addEventListener("change", function () { apply(saved()); });

  document.addEventListener("DOMContentLoaded", function () {
    apply(saved());
    document.querySelectorAll("[data-theme-choice]").forEach(function (button) {
      button.addEventListener("click", function () {
        var choice = button.getAttribute("data-theme-choice");
        try { localStorage.setItem(KEY, choice); } catch (error) { /* private mode: the theme lasts for this page */ }
        apply(choice);
      });
    });

    var toggle = document.querySelector("[data-menu-toggle]");
    if (toggle) {
      toggle.addEventListener("click", function () {
        setMenu(toggle.getAttribute("aria-expanded") !== "true");
      });
    }
    var scrim = document.querySelector("[data-menu-close]");
    if (scrim) { scrim.addEventListener("click", function () { setMenu(false); }); }
    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && document.documentElement.hasAttribute("data-menu-open")) {
        setMenu(false);
        if (toggle) { toggle.focus(); }
      }
    });

    // Native <dialog>: showModal gives focus trapping, Escape and the ::backdrop for free.
    document.querySelectorAll("[data-dialog-open]").forEach(function (button) {
      button.addEventListener("click", function () {
        var dialog = document.getElementById(button.getAttribute("data-dialog-open"));
        if (dialog && typeof dialog.showModal === "function") { dialog.showModal(); }
      });
    });

    // The kind decides which fields mean something: a built-in provider needs only its key. A hidden
    // field is also disabled so a value typed before switching kinds is never posted. Without JS all stay.
    document.querySelectorAll("[data-kind-select]").forEach(function (select) {
      var form = select.form;
      if (!form) { return; }
      var field = form.querySelector("[data-base-url-field]");
      var url = form.querySelector("[data-base-url-input]");
      var key = form.querySelector("[data-key-input]");
      function applyKind() {
        var option = select.options[select.selectedIndex];
        if (!option) { return; }
        var mode = option.getAttribute("data-base-url");
        if (field && url) {
          field.hidden = mode === "hidden";
          url.disabled = mode === "hidden";
          url.required = mode === "required";
        }
        if (key) { key.required = option.getAttribute("data-key-required") === "true"; }
      }
      select.addEventListener("change", applyKind);
      applyKind();
    });

    // Full-page form posts show the departing-train bar on the pressed button until the next page arrives.
    document.addEventListener("submit", function (event) {
      var form = event.target;
      if (!(form instanceof HTMLFormElement) || form.method === "dialog") { return; }
      var submitter = event.submitter;
      if (submitter && submitter.getAttribute("formmethod") === "dialog") { return; }
      if (submitter && submitter.classList.contains("button")) { submitter.classList.add("is-pending"); }
    });
    // Coming back through the history cache must not leave a bar running.
    window.addEventListener("pageshow", function () {
      document.querySelectorAll(".is-pending").forEach(function (button) { button.classList.remove("is-pending"); });
    });
  });
})();
