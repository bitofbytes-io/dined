// Where should we eat?: the result card flashes through a few other names
// from the hat, slowing down, before it settles on the pick.
function runSpinReel() {
  var card = document.querySelector("[data-spin-result]");
  if (!card || card.dataset.spinReelDone) return;
  card.dataset.spinReelDone = "1";
  var name = card.querySelector("[data-spin-name]");
  var names = [];
  try {
    names = JSON.parse(card.dataset.spinReel || "[]");
  } catch (error) {
    names = [];
  }
  var reduceMotion = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (!name || !names.length || reduceMotion) return;
  var pick = name.textContent;
  // The link goes to the pick, so it is not a link while it shows other names.
  var href = name.getAttribute("href");
  var frames = names.length < 6 ? names.concat(names) : names;
  var frame = 0;
  name.removeAttribute("href");
  card.classList.add("is-spinning");
  card.setAttribute("aria-busy", "true");
  function step() {
    if (frame < frames.length) {
      name.textContent = frames[frame];
      frame += 1;
      window.setTimeout(step, 45 + frame * 12);
      return;
    }
    name.textContent = pick;
    if (href) name.setAttribute("href", href);
    card.classList.remove("is-spinning");
    card.removeAttribute("aria-busy");
  }
  step();
}

document.addEventListener("htmx:load", runSpinReel);
runSpinReel();
