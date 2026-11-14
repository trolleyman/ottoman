# Terminal font

`iosevka-term-nerd-font-mono-regular.woff2` is a full, unsubsetted WOFF2 conversion of
`IosevkaTermNerdFontMono-Regular.ttf` from the user's `~/.dotfiles/fonts`.
The font reports `Version 33.2.1;Nerd Fonts 3.4.0`; the dotfiles version manifest
records `ryanoasis/nerd-fonts@v3.4.0`.

Source TTF SHA-256: `7ecc07f7af0d9d3829a6d6a1beeeafeda0074332d94ce8ba49c7f675b63b770e`

The Term Mono variant uses terminal spacing and single-cell Nerd Font icons.
All 17,827 mapped code points, including supplementary private-use icons, and
all horizontal glyph metrics are retained. The font is distributed under the
SIL Open Font License 1.1; see `OFL-Iosevka.txt`. Copyright and licence metadata
are also preserved in the binary.

To regenerate with a supplied source TTF, install `fonttools[woff]==4.68.0`
and `brotli==1.2.1` in a local virtualenv, then run:

```sh
python web/scripts/build-iosevka-font.py /path/to/IosevkaTermNerdFontMono-Regular.ttf
```

Normal builds use the committed WOFF2 and require no Python tooling or host fonts.
