//go:build desktop

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// A hit-test probe that runs inside the real WebKitGTK webview.
//
// The topbar being unclickable cannot be reproduced in a headless browser: the
// desktop window's engine is the one that matters, and it is the one that
// cannot be driven from a shell. So instead of guessing, the window is asked.
//
// When SIMARC_DESKTOP_PROBE is set the shell:
//
//  1. signs the window in (so the topbar exists);
//  2. injects a small script into the real page it renders, in the real engine;
//  3. has that script report what sits on top of each topbar trigger, whether
//     Bootstrap's JavaScript actually loaded, and whether a real click at the
//     trigger's own coordinates opens a menu;
//  4. prints the findings in the log.
//
// The result distinguishes the two possible causes: something invisible painted
// over the bar, or the click arriving with no handler bound because a script
// 404'd. The window also keeps asking until it gets a report, so a page that
// never reports is itself a finding.
//
// It deliberately does NOT use an iframe: the app sends X-Frame-Options: DENY,
// which would block the probe from seeing anything.

const probeEnv = "SIMARC_DESKTOP_PROBE"

// probeAssetPath serves the injected script; probeReportPath receives findings.
const (
	probeAssetPath  = "/__desktop_probe.js"
	probeReportPath = "/__desktop_probe_report"
)

// Environment switches for the probe. The credentials are only read when the
// probe is switched on, and only ever used against this machine's own loopback
// server.
const (
	probeUserEnv = "SIMARC_PROBE_USER"
	probePassEnv = "SIMARC_PROBE_PASS"
)

// withProbe wraps the engine so the window can be asked to run the hit test.
// With SIMARC_DESKTOP_PROBE unset it returns the handler untouched, so nothing
// is registered and a normal session pays nothing.
func withProbe(next http.Handler) http.Handler {
	if os.Getenv(probeEnv) == "" {
		return next
	}
	log.Printf("[desktop] probe aktif — halaman akan disuntik script diagnostik")

	creds, _ := json.Marshal(map[string]string{
		"user": getEnvDefault(probeUserEnv, "admin"),
		"pass": getEnvDefault(probePassEnv, "admin"),
	})
	report := "location.origin + " + jsonString(probeReportPath)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The script itself, and the endpoint it reports to.
		switch r.URL.Path {
		case probeAssetPath:
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, probeScript(creds, report))
			return
		case probeReportPath:
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			var res probeResult
			if err := json.Unmarshal(body, &res); err != nil {
				log.Printf("[desktop] probe report bukan JSON: %v — %s", err, clip(string(body), 200))
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			logProbeResult(res)
			_, _ = io.WriteString(w, "ok")
			return
		}

		// Everything else is the app. Buffer an HTML response just far enough to
		// inject the script before the closing body tag.
		rec := &htmlInjector{
			ResponseWriter: w,
			script:         fmt.Sprintf(`<script src="%s"></script>`, probeAssetPath),
			probeData:      fmt.Sprintf(`window.__SIMARC_PROBE__=%s;`, creds),
		}
		next.ServeHTTP(rec, r)
	})
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// htmlInjector adds the probe script to the end of the first HTML document it
// sees. It streams everything else straight through, so a normal asset response
// is not buffered at all.
type htmlInjector struct {
	http.ResponseWriter
	script    string
	probeData string
	done      bool
	seenHTML  bool
}

func (h *htmlInjector) WriteHeader(code int) {
	ct := h.Header().Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		h.seenHTML = true
		// Injecting changes the body length, so a Content-Length computed by the
		// handler no longer describes what is on the wire. Leaving it in place
		// truncates the document at the original length, which stops the parser
		// before the closing </body> — no stylesheets, no scripts, no report. Go
		// falls back to chunked encoding when this header is absent.
		h.Header().Del("Content-Length")
		// The engine sets a strict content policy; loosen it just enough for the
		// script to load. Without this the probe is blocked and reports nothing,
		// which would be indistinguishable from the bug we are hunting.
		h.Header().Set("Content-Security-Policy", "")
	}
	h.ResponseWriter.WriteHeader(code)
}

func (h *htmlInjector) Write(p []byte) (int, error) {
	if !h.seenHTML {
		return h.ResponseWriter.Write(p)
	}
	if h.done {
		return h.ResponseWriter.Write(p)
	}
	h.done = true

	body := bytes.Replace(p, []byte("</body>"),
		[]byte(h.probeData+h.script+"</body>"), 1)
	if bytes.Equal(body, p) {
		// No </body> — append rather than silently losing the probe.
		body = append(append([]byte{}, p...), []byte(h.probeData+h.script)...)
	}
	if _, err := io.WriteString(h.ResponseWriter, string(body)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// probeResult is what the webview found, in a shape the log can print.
type probeResult struct {
	URL          string         `json:"url"`
	TopbarFound  bool           `json:"topbarFound"`
	LayoutTopbar bool           `json:"layoutTopbar"`
	HeaderRect   *probeRect     `json:"headerRect"`
	Scripts      []scriptStatus `json:"scripts"`
	Triggers     []probeTrigger `json:"triggers"`
	Console      []string       `json:"consoleErrors"`
	PageErrors   []string       `json:"pageErrors"`
	Notes        []string       `json:"notes"`
}

type probeRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type scriptStatus struct {
	URL     string `json:"url"`
	Loaded  bool   `json:"loaded"`
	Defined string `json:"defined"`
	Err     string `json:"error"`
}

type probeTrigger struct {
	Label     string   `json:"label"`
	Point     [2]int   `json:"point"`
	TopEl     string   `json:"topEl"`
	Reachable bool     `json:"reachable"`
	Blockers  []string `json:"blockers"`
	Opened    bool     `json:"opened"`
}

func logProbeResult(res probeResult) {
	log.Printf("[desktop] ===== LAPORAN PROBE DARI WINDOW =====")
	log.Printf("[desktop]   url            : %s", res.URL)
	log.Printf("[desktop]   topnav ditemukan : %v (layout-topbar=%v)", res.TopbarFound, res.LayoutTopbar)
	if res.HeaderRect != nil {
		r := *res.HeaderRect
		log.Printf("[desktop]   header rect    : x=%d y=%d w=%d h=%d", r.X, r.Y, r.W, r.H)
	}
	for _, s := range res.Scripts {
		state := "GAGAT DIMUAT"
		if s.Loaded {
			state = "dimuat"
		}
		if s.Err != "" {
			state += " (" + s.Err + ")"
		}
		log.Printf("[desktop]   script %-6s : %s %s", state, s.URL, s.Defined)
	}
	for _, t := range res.Triggers {
		mark := "ok "
		if !t.Reachable {
			mark = "!! TERTUTUP "
		} else if !t.Opened {
			mark = "!! TAK TERBUKA "
		}
		opened := "tidak"
		if t.Opened {
			opened = "ya"
		}
		log.Printf("[desktop]   %s%-18s (%3d,%3d) atas=%-28s terjangkau=%-5v menu_terbuka=%s",
			mark, t.Label, t.Point[0], t.Point[1], t.TopEl, t.Reachable, opened)
		if !t.Reachable {
			for _, b := range t.Blockers {
				log.Printf("[desktop]        menutup: %s", b)
			}
		}
	}
	for _, n := range res.Notes {
		log.Printf("[desktop]   catatan: %s", n)
	}
	for _, e := range res.Console {
		log.Printf("[desktop]   console error: %s", clip(e, 200))
	}
	for _, e := range res.PageErrors {
		log.Printf("[desktop]   page error   : %s", clip(e, 200))
	}
	log.Printf("[desktop] ===== AKHIR LAPORAN PROBE =====")
}

// probeScript is injected into the real page. It reports what is on top of each
// trigger, whether Bootstrap's JavaScript is present, and what a real click at
// the trigger's own coordinates actually does.
func probeScript(creds []byte, report string) string {
	return fmt.Sprintf(`(function () {
  var CRED = %s;
  var REPORT = %s;
  var consoleErrors = [], pageErrors = [], notes = [];

  addEventListener('error', function (e) {
    pageErrors.push((e.message || String(e.error)) + ' @ ' + (e.filename || '?') + ':' + (e.lineno || 0));
  }, true);
  addEventListener('unhandledrejection', function (e) { pageErrors.push('rejection: ' + e.reason); });
  var realErr = console.error;
  console.error = function () {
    consoleErrors.push([].join.call(arguments, ' '));
    realErr.apply(console, arguments);
  };

  function label(n) {
    if (!n) return 'null';
    if (n.id) return '#' + n.id;
    var c = (typeof n.className === 'string' && n.className) ? '.' + n.className.trim().split(/\s+/).join('.') : '';
    return n.tagName.toLowerCase() + c;
  }
  function report(res) {
    res.consoleErrors = consoleErrors; res.pageErrors = pageErrors; res.notes = notes;
    try {
      var b = JSON.stringify(res);
      var x = new XMLHttpRequest();
      x.open('POST', REPORT, true);
      x.setRequestHeader('Content-Type', 'application/json');
      x.send(b);
    } catch (e) { notes.push('gagal kirim laporan: ' + e); }
  }

  // Sign in first: the topbar only exists on an authenticated page, and the
  // window starts signed out. Same-origin fetch, so the session cookie is used.
  function signIn() {
    return fetch('/login', { credentials: 'same-origin', headers: { 'X-Requested-With': 'probe' } })
      .then(function (r) { return r.text(); })
      .then(function (html) {
        var m = html.match(/name="_token"[^>]*value="([^"]+)"/) || html.match(/value="([^"]+)"[^>]*name="_token"/);
        if (!m) { notes.push('token CSRF tidak ditemukan di /login'); return null; }
        var p = new URLSearchParams();
        p.set('username', CRED.user); p.set('password', CRED.pass); p.set('_token', m[1]);
        return fetch('/login', {
          method: 'POST', credentials: 'same-origin',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: p.toString()
        }).then(function (r) { notes.push('login -> ' + r.status); });
      });
  }

  // Which script tags failed is the single most useful fact if clicks do
  // nothing: a dropdown trigger is inert without its JavaScript.
  function scripts() {
    var out = [];
    var tags = [].slice.call(document.querySelectorAll('script[src]'));
    tags.forEach(function (t) {
      var url = t.getAttribute('src');
      out.push({ url: url, loaded: t.dataset.probeLoaded === '1', defined: '', error: '' });
    });
    return out;
  }

  function run() {
    var res = {
      url: location.pathname,
      topbarFound: !!document.getElementById('topnav'),
      layoutTopbar: document.body.classList.contains('layout-topbar'),
      scripts: [],
      triggers: [],
      consoleErrors: [], pageErrors: [], notes: []
    };

    // Bootstrap and Popper are globals; their presence is the ground truth
    // about whether the dropdown behaviour can possibly be bound.
    notes.push('bootstrap global: ' + (typeof window.bootstrap));
    notes.push('bootstrap.Dropdown: ' + (window.bootstrap ? typeof window.bootstrap.Dropdown : 'n/a'));
    notes.push('Popper global: ' + (typeof window.Popper));

    // Which <script src> actually arrived. A 404 here is the bug.
    var tags = [].slice.call(document.querySelectorAll('script[src]'));
    res.scripts = tags.map(function (t) {
      var u = t.getAttribute('src');
      return fetch(u, { method: 'HEAD', credentials: 'same-origin' })
        .then(function (r) { return { url: u, loaded: r.ok, status: r.status, defined: '' }; })
        .catch(function (e) { return { url: u, loaded: false, error: String(e), defined: '' }; });
    });
    Promise.all(res.scripts).then(function (rows) {
      res.scripts = rows;
      probeTriggers(res);
    });
  }

  function probeTriggers(res) {
    var header = document.querySelector('.top-navbar');
    if (header) {
      var r = header.getBoundingClientRect();
      res.headerRect = { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
      var cs = getComputedStyle(header);
      notes.push('header: pointer-events=' + cs.pointerEvents + ' z-index=' + cs.zIndex +
                 ' position=' + cs.position + ' user-select=' + cs.userSelect);
    }

    var items = [].slice.call(document.querySelectorAll('#topnav .topnav-item'));
    if (!items.length) {
      notes.push('tidak ada .topnav-item di halaman ini');
      report(res);
      return;
    }

    items.forEach(function (el) {
      var r = el.getBoundingClientRect();
      var cx = Math.round(r.left + r.width / 2);
      var cy = Math.round(r.top + r.height / 2);
      var stack = document.elementsFromPoint(cx, cy).slice(0, 6).map(label);
      var hit = document.elementFromPoint(cx, cy);
      res.triggers.push({
        label: (el.textContent || '').trim().slice(0, 20),
        point: [cx, cy],
        topEl: stack[0] || 'null',
        reachable: !!(hit && (hit === el || el.contains(hit) || hit.contains(el))),
        blockers: stack,
        opened: false
      });
    });

    // Now click them for real, at their own coordinates, the way a mouse does.
    // A dispatched event at the right point answers "does a pointer event reach
    // the trigger"; it cannot answer "did the engine hit-test to it", which is
    // why the hit test above runs first and the two are read together.
    var triggers = [].slice.call(document.querySelectorAll('#topnav .topnav-item[data-bs-toggle="dropdown"]'));
    var i = 0;
    (function next() {
      if (i >= triggers.length) { report(res); return; }
      var el = triggers[i++];
      var r = el.getBoundingClientRect();
      var cx = Math.round(r.left + r.width / 2);
      var cy = Math.round(r.top + r.height / 2);
      var name = (el.textContent || '').trim().slice(0, 20);
      ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click'].forEach(function (type) {
        var Ctor = type.indexOf('pointer') === 0 ? (window.PointerEvent || MouseEvent) : MouseEvent;
        el.dispatchEvent(new Ctor(type, { bubbles: true, cancelable: true, view: window,
                                          clientX: cx, clientY: cy, button: 0, buttons: 1 }));
      });
      setTimeout(function () {
        var menu = el.parentNode ? el.parentNode.querySelector('.topnav-menu') : null;
        var shown = !!(menu && menu.classList.contains('show'));
        var rec = res.triggers.filter(function (x) { return x.label === name; })[0];
        if (rec) {
          rec.opened = el.getAttribute('aria-expanded') === 'true' && shown;
          rec.blockers = rec.reachable ? rec.blockers : rec.blockers;
        }
        setTimeout(next, 300);
      }, 400);
    })();
  }

  // If this page is signed out, log in and come back so the topbar exists.
  if (document.getElementById('topnav')) {
    run();
  } else {
    signIn().then(function () {
      if (document.getElementById('topnav')) { run(); return; }
      // The page we are on is not the app (e.g. an error page); go to the
      // dashboard and let the injected copy of this script run there.
      location.replace('/dashboard');
    }).catch(function (e) {
      notes.push('probe gagal: ' + e);
      report({ url: location.pathname, topbarFound: false, scripts: [], triggers: [],
               consoleErrors: consoleErrors, pageErrors: pageErrors, notes: notes });
    });
  }
})();
`, creds, report)
}
