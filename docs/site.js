// The front page's two behaviors, and nothing else: a [copy] beside a
// command copies it, and below the station's width the station is
// scaled rather than rewrapped — it is drawn on the terminal's grid,
// and a grid rewrapped is a different picture.
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

  const room = document.querySelector('.station-room');
  const station = document.querySelector('.station');
  if (!room || !station) return;
  const fit = () => {
    const width = station.offsetWidth;
    const have = room.clientWidth;
    const scale = have < width ? have / width : 1;
    station.style.setProperty('--scale', scale.toFixed(4));
    station.style.left = Math.floor((have - width * scale) / 2) + 'px';
    room.style.height = Math.ceil(station.offsetHeight * scale) + 'px';
  };
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(fit);
  fit();
  addEventListener('resize', fit);
})();
