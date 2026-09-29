# README media

`try-omarchy-windows.gif` combines the pixel wordmark animation from
[tryomarchy.com](https://tryomarchy.com/) with the Windows product still in
`tryomarchy-site/images/try/windows.webp` at
[site commit `6595502`](https://github.com/btsouth/tryomarchy-site/commit/659550234e39ebc7d6aa81102b3eb8b143f8d665).
The wordmark was captured from the public page on September 26, 2026. The GIF
is silent, 960 × 540, and about 1.4 MB.

`try-omarchy-windows-still.jpg` is a resized copy of the same Windows still.
The site footage shows a Windows 11 session recorded before v0.5.0 and is
illustrative product media, not release acceptance evidence. Refresh it from
a verified current capture when the product appearance changes.

## Linux presentation

`try-omarchy-linux.gif` is a silent, looping 10-second introduction, combining
an animation of the official Omarchy pixel wordmark with a real Linux capture.
The README uses `try-omarchy-linux-hero.png` for reduced-motion viewing.
`try-omarchy-linux.webm` carries the same sequence in VP9 for Software’s video
gallery. Software controls playback; the README GIF loops automatically.
`try-omarchy-linux-desktop.jpg` is the full desktop capture for the listing.

The capture was taken on September 28, 2026 from candidate 4 in a separate
Ubuntu 24.04 test VM, running Omarchy with Tokyo Night and its menu open.
It contains fixture data. It is product media, not proof of physical GPU support.
The original capture is `linux-media/desktop-source.png`.

The official wordmark is preserved in `scripts/linux/media/omarchy-wordmark.svg`,
from the Try Omarchy site’s `brand/omarchy-wordmark.svg`. Omarchy’s marks remain
subject to their owners’ trademark rights; see the third-party notices.

Regenerate on devbox with Python 3, DejaVu fonts, `rsvg-convert` and FFmpeg:

```sh
python3 scripts/linux/media/render.py docs/images/linux-media/desktop-source.png docs/images
```

Publish these files on the repository’s default branch before shipping metadata
that references their public URLs. The new gallery URLs are not live yet.
