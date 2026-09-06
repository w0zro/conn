// The manual's two behaviors, and nothing else: a [copy] beside a command
// copies it, and below the desk's width the pages are scaled rather than
// rewrapped — a page is a fixed artifact, read smaller on a small screen.
(() => {
  for (const b of document.querySelectorAll('.copy')) {
    b.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(b.dataset.copy);
        b.textContent = '[copied]';
        b.classList.add('done');
        setTimeout(() => { b.textContent = '[copy]'; b.classList.remove('done'); }, 1400);
      } catch {
        b.textContent = '[select it]';
      }
    });
  }

  const fit = () => {
    const room = document.documentElement.clientWidth;
    const scale = room < 880 ? Math.min(1, (room - 16) / 816) : 1;
    document.documentElement.style.setProperty('--scale', scale.toFixed(4));
  };
  fit();
  addEventListener('resize', fit);
})();
