// The install commands on the landing page and in the footer: a copy button
// on each.
//
// An enhancement over markup that already works without it. The buttons ship
// `hidden` and are revealed only once this runs, so a reader without
// JavaScript never meets a control that does nothing.
(function () {
  "use strict";

  var DONE_MS = 1500;

  // What a card copies is exactly what it shows, one command per line. The
  // "$ " prompt is drawn by CSS (::before), so it is not in textContent and
  // never lands in the clipboard.
  function commandsOf(card) {
    var lines = [];
    card.querySelectorAll(".snoop-install__command").forEach(function (el) {
      lines.push(el.textContent.trim());
    });
    return lines.join("\n");
  }

  // Where the Clipboard API is unavailable — a page served over plain HTTP,
  // say — select the commands instead, so a manual copy takes one keystroke.
  function selectCommands(card) {
    var commands = card.querySelectorAll(".snoop-install__command");
    if (!commands.length || !window.getSelection) return;
    var range = document.createRange();
    range.setStartBefore(commands[0]);
    range.setEndAfter(commands[commands.length - 1]);
    var selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
  }

  function announce(card, button, message) {
    var status = card.querySelector(".snoop-install__status");
    if (status) status.textContent = message;
    button.classList.add("is-done");
    clearTimeout(button._snoopDone);
    button._snoopDone = setTimeout(function () {
      button.classList.remove("is-done");
      if (status) status.textContent = "";
    }, DONE_MS);
  }

  function wireCopy(card) {
    var button = card.querySelector(".snoop-install__copy");
    if (!button) return;
    button.hidden = false;
    button.addEventListener("click", function () {
      var text = commandsOf(card);
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).then(
          function () { announce(card, button, "Copied"); },
          function () { selectCommands(card); }
        );
      } else {
        selectCommands(card);
      }
    });
  }

  function init() {
    document.querySelectorAll("[data-snoop-copy]").forEach(wireCopy);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
