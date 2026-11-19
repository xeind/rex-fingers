# Assets

| File | Holds |
| --- | --- |
| `rex-fingers.icon` | The Icon Composer document: `claw.svg` as a matte Liquid Glass layer over `field.svg`, Rex's radial gradient sampled from the app icon, and `claw-dark.svg`, the claw filled with that gradient, shown only in Dark. |
| `logo-claw-glass-rex.png`, `logo-claw-glass-rex-dark.png` | Its exports at 512 px, the README logo. Regenerate with File > Export, Appearance All, 2x. |
| `logo-claw-black.svg`, `logo-claw-white.svg` | The bare mark. |
| `social-preview.png` | The dark render padded to 1280x640 for GitHub. |
| `demo-window-hints.gif` | The README demo: Alt-Shift-f over nvim, Claude Code, and a shell, 6 s. Made from a screen recording with `ffmpeg -vf "fps=5,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" -loop 0`, 2.2 MB. The clip only switches between a plain and a hinted screen, so 5 fps loses nothing; 30 fps made it 4.2 MB and slow to load. |
