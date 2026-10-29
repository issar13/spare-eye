# Frequently asked questions

## Does SpareEye require a subscription or cloud account?

No. Detection and storage run on your computer. Telegram alerts use a bot that
you create and control. Alert media is uploaded to Telegram when that feature
is enabled.

## Why is Telegram the only alerting channel?

Telegram supports outbound alerts and inbound commands through a connection
opened by the app, so SpareEye does not need a public server or webhook.

WhatsApp's official Cloud API is a poor fit for a local desktop app: proactive
messages require a Meta Business account and approved templates, while receiving
commands requires a publicly reachable HTTPS webhook. Unofficial integrations
avoid those requirements by driving WhatsApp's private protocol, but introduce
terms-of-service and account-ban risk.

The `notify.Notifier` interface keeps alert delivery separate from motion
detection, so another provider can be added without rewriting the detector. See
[Adding a notifier](ARCHITECTURE.md#adding-a-notifier).

## Why does SpareEye ask me to choose a camera?

Device indices are not reliable indicators of the physical webcam. A virtual
camera may occupy index 0, and connected capture devices can move between
indices. SpareEye tests which cameras can deliver a frame, then leaves the
meaningful choice to you.

## Why are recordings stored in the user configuration directory?

Installed applications cannot normally write beside their executable, especially
under `C:\Program Files`. Per-user storage also prevents different operating
system users from sharing credentials and footage unintentionally.

See [Privacy and security](SECURITY.md#know-where-footage-goes) for the default
locations.

## Why does Windows show a SmartScreen warning?

Current Windows release binaries are unsigned. Code-signing certificates have
an ongoing cost, so Windows cannot verify a known publisher and warns before
the first launch.

## Why is macOS not supported?

Shipping a trustworthy macOS desktop build requires signing and notarization
through the Apple Developer program. Support is deferred instead of publishing
an incomplete or misleading build.

## Can SpareEye run without Telegram?

Yes. The detector still writes snapshots and clips locally when credentials are
absent or Telegram is disabled. For a one-off headless run:

```bash
go run ./apps/cli -no-telegram
```

## Can it run without a desktop?

Yes. `apps/cli` runs the detector without a window and is suitable for a
machine managed over SSH. See [Development](DEVELOPMENT.md) for building it and
run `make help` for available commands.
