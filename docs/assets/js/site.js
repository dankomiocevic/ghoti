// Progressive enhancement only. Every page works without this file.
(function () {
  "use strict";
  var root = document.documentElement;

  // Theme toggle. Dark by default; a saved choice is applied by the inline snippet in <head>.
  var toggle = document.querySelector(".theme-toggle");
  if (toggle) {
    toggle.addEventListener("click", function () {
      // Dark is the default; only "light" is stored as an attribute.
      var next = root.getAttribute("data-theme") === "light" ? "dark" : "light";
      if (next === "light") root.setAttribute("data-theme", "light");
      else root.removeAttribute("data-theme");
      try { localStorage.setItem("ghoti-theme", next); } catch (e) { /* storage unavailable */ }
    });
  }

  // Reveal on scroll.
  var reveals = document.querySelectorAll(".reveal");
  if ("IntersectionObserver" in window) {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          entry.target.classList.add("is-visible");
          io.unobserve(entry.target);
        }
      });
    }, { rootMargin: "0px 0px -8% 0px", threshold: 0.08 });
    reveals.forEach(function (el) { io.observe(el); });
  } else {
    reveals.forEach(function (el) { el.classList.add("is-visible"); });
  }

  // Pointer-following glow on cards.
  document.querySelectorAll(".card").forEach(function (card) {
    card.addEventListener("pointermove", function (ev) {
      var r = card.getBoundingClientRect();
      card.style.setProperty("--mx", (ev.clientX - r.left) + "px");
      card.style.setProperty("--my", (ev.clientY - r.top) + "px");
    });
  });

  // Copy buttons on code blocks.
  if (navigator.clipboard) {
    document.querySelectorAll("pre").forEach(function (pre) {
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "copy-btn";
      btn.textContent = "Copy";
      btn.addEventListener("click", function () {
        var code = pre.querySelector("code") || pre;
        var text = Array.prototype.map.call(code.childNodes, function (n) {
          // Skip prompt markers such as "$ " so the copied text runs as-is.
          return n.nodeType === 1 && n.classList.contains("p") ? "" : n.textContent;
        }).join("");
        navigator.clipboard.writeText(text.replace(/\n$/, "")).then(function () {
          btn.textContent = "Copied";
          setTimeout(function () { btn.textContent = "Copy"; }, 1400);
        });
      });
      pre.appendChild(btn);
    });
  }

  // Highlight the current section in the "On this page" list.
  var tocLinks = document.querySelectorAll(".toc a");
  if (tocLinks.length && "IntersectionObserver" in window) {
    var map = {};
    tocLinks.forEach(function (a) { map[a.getAttribute("href").slice(1)] = a; });
    var headings = Object.keys(map).map(function (id) { return document.getElementById(id); }).filter(Boolean);
    var tio = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          tocLinks.forEach(function (a) { a.classList.remove("active"); });
          var link = map[entry.target.id];
          if (link) link.classList.add("active");
        }
      });
    }, { rootMargin: "0px 0px -75% 0px" });
    headings.forEach(function (h) { tio.observe(h); });
  }

  // Animated replay of the homepage transcript. The static transcript stays
  // in the page and is what you get without JavaScript or with reduced motion.
  var demo = document.querySelector("[data-demo]");
  var timelineEl = document.getElementById("demo-timeline");
  var reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (demo && timelineEl && !reduce) {
    var timeline;
    try { timeline = JSON.parse(timelineEl.textContent); } catch (e) { timeline = null; }
    if (timeline) initDemo(demo, timeline);
  }

  function initDemo(win, steps) {
    var panes = {};
    win.querySelectorAll("[data-pane]").forEach(function (pane) {
      var code = pane.querySelector("pre code");
      var pre = pane.querySelector("pre");
      // Keep the layout stable: reserve the height of the full transcript.
      pre.style.minHeight = pre.offsetHeight + "px";
      panes[pane.getAttribute("data-pane")] = { code: code, pane: pane };
    });
    var notes = document.querySelectorAll(".demo-notes [data-note]");
    var toggle = win.querySelector(".demo-toggle");
    var copyBtns = win.querySelectorAll(".copy-btn");
    copyBtns.forEach(function (b) { b.hidden = true; });
    win.classList.add("is-animated");
    win.setAttribute("aria-label", "Animated replay of the transcript");

    var paused = false, visible = false, timer = null, queue = [], qi = 0;

    function build() {
      queue = [];
      steps.forEach(function (step) {
        queue.push({ note: step.note });
        step.lines.forEach(function (l) { queue.push({ panes: l[0].split(""), kind: l[1], text: l[2] }); });
        queue.push({ wait: 900 });
      });
      queue.push({ wait: 3200 }, { reset: true });
      qi = 0;
    }

    function clear() {
      Object.keys(panes).forEach(function (k) { panes[k].code.textContent = ""; });
      notes.forEach(function (n) { n.classList.remove("is-active"); });
    }

    function lineEl(kind, text) {
      var span = document.createElement("span");
      span.className = "demo-line";
      if (kind === "c") {
        span.innerHTML = '<span class="c"></span>';
        span.firstChild.textContent = text;
      } else {
        var cls = kind === ">" ? "in" : ({ v: "v", e: "e", a: "a" })[text.charAt(0)] || "v";
        span.innerHTML = '<span class="p"></span><span class="' + cls + '"></span>';
        span.firstChild.textContent = kind + " ";
      }
      return span;
    }

    function schedule(fn, ms) { timer = setTimeout(fn, ms); }

    function next() {
      timer = null;
      if (paused || !visible) return;
      if (qi >= queue.length) { build(); }
      var item = queue[qi++];
      if (item.reset) { clear(); return schedule(next, 400); }
      if (item.wait) return schedule(next, item.wait);
      if (item.note) {
        notes.forEach(function (n) { n.classList.toggle("is-active", n.getAttribute("data-note") === String(item.note)); });
        return schedule(next, 50);
      }
      var targets = item.panes.map(function (k) { return panes[k]; });
      var els = targets.map(function (t) {
        var el = lineEl(item.kind, item.text);
        if (t.code.childNodes.length) t.code.appendChild(document.createTextNode("\n"));
        t.code.appendChild(el);
        return el;
      });
      if (item.kind === ">") {
        // Type the command one character at a time.
        var target = els[0].lastChild, i = 0;
        els[0].classList.add("typing");
        (function type() {
          if (paused || !visible) { timer = setTimeout(type, 200); return; }
          target.textContent = item.text.slice(0, ++i);
          if (i < item.text.length) { timer = setTimeout(type, 38); }
          else { els[0].classList.remove("typing"); schedule(next, 320); }
        })();
      } else {
        els.forEach(function (el) {
          var body = el.lastChild;
          body.textContent = item.text;
          el.classList.add("arrive");
        });
        schedule(next, item.kind === "c" ? 260 : 520);
      }
    }

    function start() { if (!timer && !paused && visible) next(); }

    toggle.hidden = false;
    toggle.addEventListener("click", function () {
      paused = !paused;
      toggle.textContent = paused ? "Play" : "Pause";
      toggle.setAttribute("aria-pressed", String(paused));
      if (!paused) start();
    });

    clear();
    build();
    if ("IntersectionObserver" in window) {
      new IntersectionObserver(function (entries) {
        visible = entries[0].isIntersecting;
        if (visible) start();
      }, { threshold: 0.35 }).observe(win);
    } else {
      visible = true;
      start();
    }
  }
})();
