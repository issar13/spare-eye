# Privacy and security

SpareEye records whatever its camera can see. Use it only where recording is
lawful and expected.

## Get consent

Tell everyone using a monitored space that they may be recorded and that alerts
are uploaded to Telegram. Laws differ by location, and recording people without
their knowledge or consent may be illegal even on private property.

Do not point SpareEye into a neighbour's property or any place where people
reasonably expect privacy.

## Know where footage goes

Snapshots and clips are written to local disk. Telegram is optional: only when
you connect it is alert media also uploaded to Telegram's servers. Desktop
notifications are handed to your operating system and never leave the computer,
though the notification can show the snapshot on screen, so switch them off
(`notifications.desktop`) if the screen is visible to other people. SpareEye does
not operate its own cloud service.

By default, local media is retained for 30 days and old recordings are removed
at startup. A retention value of `0` keeps everything and can eventually fill
the disk. See [Configuration](CONFIGURATION.md#storage).

Default locations:

| Data | Linux | Windows |
|---|---|---|
| Settings | `~/.config/spareeye/config.json` | `%AppData%\spareeye\config.json` |
| Snapshots | `~/.config/spareeye/media/snapshots/` | `%AppData%\spareeye\media\snapshots\` |
| Clips | `~/.config/spareeye/media/videos/` | `%AppData%\spareeye\media\videos\` |
| Log | `~/.config/spareeye/spareeye.log` | `%AppData%\spareeye\spareeye.log` |

The operating system's user configuration directory is used when it differs
from these defaults.

On the first run after an upgrade, SpareEye moves the existing configuration
directory, including credentials and recordings, to its current location. An
existing `spareeye` directory is never overwritten.

## Protect the bot token

The Telegram bot token is a password. Anyone who obtains it can call Telegram's
API as the bot, read pending messages, and potentially receive footage or issue
commands.

- Never commit a real token or paste one into an issue.
- Keep `config.json` private to the operating-system user.
- Revoke a token immediately with BotFather if it may have leaked.
- Use one bot per SpareEye installation.

SpareEye accepts commands only from the configured chat ID, but that restriction
does not make a leaked bot token safe. See
[Telegram setup](SETUP-TELEGRAM.md#your-bot-token-is-a-credential).

## Protect the computer

Anyone with access to the operating-system account can view locally stored
footage. Use a strong login password, disk encryption, and appropriate file
permissions. Keep the operating system and camera drivers updated.

Configuration saves use an owner-only file and an atomic replacement so an
interrupted save does not leave a partially written credentials file.

## Reporting a vulnerability

Do not include credentials, private footage, or identifying log data in a public
issue. Share the minimum reproducible detail and revoke any credential exposed
during investigation.
