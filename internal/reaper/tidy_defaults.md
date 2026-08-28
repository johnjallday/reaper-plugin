# Studio conventions

<!-- ori-reaper-tidy-conventions:1 -->

Project Tidy reads this file before it proposes cosmetic cleanup. Edit the RGB
values, matching words, and naming preferences to fit this studio. Unknown track
roles are intentionally left uncolored.

## Track role colors

Use `role = red, green, blue` with channels from 0 to 255.

```text
# role = RGB
# matching words are case-insensitive whole words or common abbreviations

drums = 220, 68, 64
bass = 230, 145, 56
guitars = 76, 175, 80
keys = 142, 94, 201
vocals = 55, 126, 230
bus_fx = 45, 168, 160
reference = 128, 138, 150
```

## Track role matching words

```text
drums: drum, drums, kick, snare, hat, hats, tom, toms, percussion, perc
bass: bass, sub
guitars: guitar, guitars, guit, gtr
keys: keys, keyboard, piano, organ, synth
vocals: vocal, vocals, vox, voice, lead vox, backing vox, bgv
bus_fx: bus, buss, aux, fx, reverb, delay, verb
reference: reference, ref, rough, demo
```

Keep one `role: words` line for each role listed in the color block.

## Marker and region naming

```text
case = Title Case
numbering = Space Before Number
sections = Intro, Verse, Pre-Chorus, Chorus, Bridge, Breakdown, Solo, Outro
```

Defaults mean `chorus` becomes `Chorus`, `verse2` becomes `Verse 2`, and section
names should use the vocabulary above when they clearly match. Tidy renames only
existing markers/regions and deletes only exact-position duplicates; it never
creates arrangement markers.
