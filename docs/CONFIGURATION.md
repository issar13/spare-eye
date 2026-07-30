# Configuration reference

Every setting in `config.json`, what it means, and what it defaults to.

Almost all of these are editable from the desktop app. This file is the reference
for what each one does, and for editing the file directly on a headless machine.

See [the README](../README.md#where-your-data-lives) for where `config.json`
lives on each platform.

Durations are readable strings like `"15s"` and `"5m0s"`, so the file stays
sensible to edit by hand. Any field you leave out keeps its default, so an older
`config.json` remains loadable after an upgrade.

## Telegram

| Field | Default | Meaning |
|---|---|---|
| `telegram.botToken` | empty | Your bot's token from @BotFather |
| `telegram.chatID` | empty | The only chat that receives alerts and may send commands |
| `telegram.enabled` | `true` | Turn alerting off without deleting your credentials |

## Detection

| Field | Default | Meaning |
|---|---|---|
| `detection.cameraIndex` | `0` | Device index to capture from. Confirm yours with `-list-cameras` before trusting the default |
| `detection.nightStartMinutes` | `1140` (19:00) | Start of the night profile, minutes since midnight |
| `detection.nightEndMinutes` | `360` (06:00) | End of the night profile, minutes since midnight |
| `detection.motionPersist` | `"5s"` | How long movement must continue before a clip starts |
| `detection.prebufferLead` | `"3s"` | How much footage from *before* the trigger is prepended to a clip |
| `detection.clipDuration` | `"15s"` | Length of an automatic clip. Manual clips ignore this and run until `/stop` |
| `detection.previewFPS` | `12` | How often a second the live view is refreshed, 1 to 60. A ceiling, not a rate: a slower camera is shown at its own speed |
| `detection.previewQuality` | `80` | Picture quality of the live view, 30 to 95. Lower is blurrier and reaches the window sooner |

Both preview fields change only what you see on screen. Snapshots, clips, and
anything sent to Telegram are always encoded at full quality regardless. Lowering
them buys back CPU on a slow machine without making detection less sensitive.

The desktop app edits these two on the **Live view** screen, under the picture
they affect, rather than with the motion settings. Saving applies at once: the
camera keeps running and a clip in progress is not interrupted.

## Sensitivity profiles

`detection.day` and `detection.night` hold the same four fields, applied
depending on the time of day.

| Field | Day | Night | Meaning |
|---|---|---|---|
| `minimumContourArea` | `8000` | `9000` | Pixel area a moving region must exceed to count. Raise it to ignore smaller movement |
| `thresholdValue` | `30` | `30` | Per-pixel brightness change treated as movement. Raise it to ignore gradual light changes |
| `sampleInterval` | `"1s"` | `"1s"` | Gap between frames examined for motion while idle |
| `alertCooldown` | `"5m0s"` | `"5m0s"` | Minimum gap between snapshot alerts, so one long event is not a stream of messages |

Night is slightly less sensitive by default: a dim, noisy image produces more
spurious pixel change, so a larger region is required before it counts.

The night boundaries are minutes since midnight rather than whole hours, so a
schedule turning over at `21:30` is expressible instead of being rounded to
something you did not mean. Multiply hours by 60 and add the minutes. A window
that wraps past midnight is handled correctly, and setting start equal to end
disables the night profile entirely.

## Storage

| Field | Default | Meaning |
|---|---|---|
| `storage.mediaDirectory` | per-user media directory | Where snapshots and clips are written |
| `storage.retentionDays` | `30` | Recordings older than this are deleted at startup. `0` keeps everything, which will eventually fill the disk |

Clips are written at the frame rate measured from the camera on startup, not a
configured one. A guessed rate is what makes recordings play back too fast or too
slow, so the rate is not exposed as a setting.

## Startup

| Field | Default | Meaning |
|---|---|---|
| `startup.launchAtLogin` | `false` | Register the app so it opens when you log in |
| `startup.watchOnLaunch` | `false` | Begin watching as soon as the app opens, however it was opened |

The two are independent. `launchAtLogin` alone opens the app and leaves the
camera off until you press Start watching; `watchOnLaunch` alone starts watching
whenever you open the app yourself. Both are off on a fresh install and on a
`config.json` that predates them.

Editing `startup.launchAtLogin` by hand only records the preference. The
registration itself is made when you save the switch on the **Setup** screen. If
the two disagree, because security software removed the registration or the app
was moved after it was made, Setup shows what your computer will really do and
offers to put it back.

When your computer opens the app at login it passes `--started-at-login` and the
window starts minimised, so a login is not interrupted. Opening the app yourself
gives you a normal window.

The registration is per-user, needs no administrator rights, and can be removed
by hand:

| Platform | Where it lives |
|---|---|
| Windows | The `SpareEye` value under `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`, editable with `regedit` or Task Manager's Startup apps tab |
| Linux | `~/.config/autostart/spareeye.desktop`, or `$XDG_CONFIG_HOME/autostart/`. Delete the file, or use your desktop's startup-applications screen |
| macOS | Not supported. The switch reports that rather than pretending to work |

The headless CLI ignores both: running it is already a request to watch, and
starting it at login is a job for systemd or your shell profile.

