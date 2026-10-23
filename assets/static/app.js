(function () {
  'use strict';
  var body = document.body;
  // iPadOS reports a desktop Safari UA; touch points are the only tell.
  if (body.dataset.platform === 'desktop' && /Macintosh/.test(navigator.userAgent) && navigator.maxTouchPoints > 1) {
    body.dataset.platform = 'ios';
  }
  // Click beacons: fire-and-forget, never block or alter navigation.
  document.addEventListener('click', function (ev) {
    try {
      var el = ev.target.closest && ev.target.closest('[data-ev]');
      if (!el || !navigator.sendBeacon) return;
      navigator.sendBeacon('/e', JSON.stringify({ e: el.dataset.ev, t: body.dataset.type, p: body.dataset.platform }));
    } catch (err) { /* statistics must never break the page */ }
  });
})();
