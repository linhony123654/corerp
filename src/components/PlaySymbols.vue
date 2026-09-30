<template>
  <svg xmlns="http://www.w3.org/2000/svg" width="0" height="0" class="play-symbols" aria-hidden="true"><defs>
    <symbol id="play-i-menu" viewBox="0 0 24 24"><path d="M4 6h16M4 12h12M4 18h8"/></symbol>
    <symbol id="play-i-more" viewBox="0 0 24 24"><circle cx="12" cy="5" r="1" fill="currentColor"/><circle cx="12" cy="12" r="1" fill="currentColor"/><circle cx="12" cy="19" r="1" fill="currentColor"/></symbol>
    <symbol id="play-i-close" viewBox="0 0 24 24"><path d="m6 6 12 12M18 6 6 18"/></symbol>
    <symbol id="play-i-plus" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></symbol>
    <symbol id="play-i-down" viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></symbol>
    <symbol id="play-i-right" viewBox="0 0 24 24"><path d="m9 6 6 6-6 6"/></symbol>
    <symbol id="play-i-back" viewBox="0 0 24 24"><path d="m14 6-6 6 6 6"/></symbol>
    <symbol id="play-i-send" viewBox="0 0 24 24"><path d="M12 19V5m-6 6 6-6 6 6"/></symbol>
    <symbol id="play-i-continue" viewBox="0 0 24 24"><path d="m6 6 8 6-8 6V6ZM18 6v12"/></symbol>
    <symbol id="play-i-stop" viewBox="0 0 24 24"><rect x="7" y="7" width="10" height="10" rx="1.7" fill="currentColor" stroke="none"/></symbol>
    <symbol id="play-i-book" viewBox="0 0 24 24"><path d="M3 4.5c3-.8 6-.3 9 1.5 3-1.8 6-2.3 9-1.5v14c-3-.8-6-.3-9 1.5-3-1.8-6-2.3-9-1.5v-14ZM12 6v14"/></symbol>
    <symbol id="play-i-location" viewBox="0 0 24 24"><path d="M19 10c0 5-7 11-7 11S5 15 5 10a7 7 0 1 1 14 0Z"/><circle cx="12" cy="10" r="2.5"/></symbol>
    <symbol id="play-i-people" viewBox="0 0 24 24"><circle cx="9" cy="8" r="3"/><path d="M3 20v-1a6 6 0 0 1 12 0v1M17 5a3 3 0 0 1 0 6m1 4a5 5 0 0 1 3 5"/></symbol>
    <symbol id="play-i-moon" viewBox="0 0 24 24"><path d="M20.8 13.1A9 9 0 0 1 10.9 3.2a9 9 0 1 0 9.9 9.9Z"/></symbol>
    <symbol id="play-i-chat" viewBox="0 0 24 24"><path d="M20 11a8 8 0 0 1-8 8H8l-5 3 1.5-6A8 8 0 1 1 20 11Z"/></symbol>
    <symbol id="play-i-world" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c5 5 5 13 0 18-5-5-5-13 0-18Z"/></symbol>
    <symbol id="play-i-memory" viewBox="0 0 24 24"><path d="M7 3h11a1 1 0 0 1 1 1v17H7a3 3 0 0 1-3-3V6a3 3 0 0 1 3-3ZM7 3v18M10 8h6M10 12h5"/></symbol>
    <symbol id="play-i-settings" viewBox="0 0 24 24"><path d="M4 6h16M4 12h16M4 18h16"/><circle cx="8" cy="6" r="2" style="fill:var(--sheet)"/><circle cx="16" cy="12" r="2" style="fill:var(--sheet)"/><circle cx="10" cy="18" r="2" style="fill:var(--sheet)"/></symbol>
    <symbol id="play-i-export" viewBox="0 0 24 24"><path d="M4 12v8h16v-8M12 3v12m-5-7 5-5 5 5"/></symbol>
    <symbol id="play-i-edit" viewBox="0 0 24 24"><path d="m4 20 1-5L16 4a2.1 2.1 0 0 1 3 3L8 19l-4 1Zm10-14 3 3"/></symbol>
    <symbol id="play-i-copy" viewBox="0 0 24 24"><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></symbol>
    <symbol id="play-i-pin" viewBox="0 0 24 24"><path d="M8 3h8l-1 7 3 3H6l3-3-1-7Zm4 10v8"/></symbol>
    <symbol id="play-i-history" viewBox="0 0 24 24"><path d="M3 10a9 9 0 1 1 2 8M3 4v6h6M12 7v6l4 2"/></symbol>
    <symbol id="play-i-refresh" viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-2.6-6.4L21 8M21 3v5h-5"/></symbol>
    <symbol id="play-i-trash" viewBox="0 0 24 24"><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v5m4-5v5"/></symbol>
    <symbol id="play-i-check" viewBox="0 0 24 24"><path d="m5 12 4 4 10-10"/></symbol>
    <symbol id="play-i-eye" viewBox="0 0 24 24"><path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/></symbol>
    <symbol id="play-i-map" viewBox="0 0 24 24"><path d="m3 5 6-2 6 2 6-2v16l-6 2-6-2-6 2V5Zm6-2v16m6-14v16"/></symbol>
    <symbol id="play-i-bag" viewBox="0 0 24 24"><path d="M8 6V4h8v2M6 8h12l2 12H4L6 8Zm2 6h8"/><path d="M8 8a4 4 0 0 0 8 0"/></symbol>
    <symbol id="play-i-clock" viewBox="0 0 24 24"><path d="M12 3a9 9 0 1 1-9 9M3.3 9.5l.2-.6M4.5 6.8l.4-.5M7 4.7l.6-.3M9.6 3.3l.6-.1"/><path class="clock-hand" d="M12 6.5V12l3.7 2.2"/></symbol>
    <symbol id="play-i-palette" viewBox="0 0 24 24"><path d="M12 3a9 9 0 0 0 0 18h1a2 2 0 0 0 1.8-2.8l-.3-.6A1.8 1.8 0 0 1 16.1 15H18a3 3 0 0 0 3-3 9 9 0 0 0-9-9Z"/><circle cx="7.5" cy="10" r=".8" fill="currentColor"/><circle cx="10" cy="6.8" r=".8" fill="currentColor"/></symbol>
  </defs></svg>
</template>
