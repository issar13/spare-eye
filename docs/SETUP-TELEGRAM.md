# Setting up Telegram

SpareEye sends alerts through a Telegram bot that **you** create. There is no
shared bot and no token built into the app, so your footage goes to your bot
and your chat and nobody else's.

The desktop app talks straight to `api.telegram.org`. It does not use a Kaa
Rada VPS or relay, does not open an inbound port, and does not configure a
webhook. Commands are collected through an outbound Telegram connection and
only the automatically registered chat is allowed to control the camera.

You need two things:

1. A **bot token**, identifying the bot the app sends messages as
2. One message to the bot, so SpareEye can identify your conversation

This takes about five minutes and needs only the Telegram app, on phone or
desktop.

## Step 1: Create the bot

1. Open Telegram and search for **@BotFather**, Telegram's official bot for
   making bots, with a blue verified checkmark next to its name.
2. Open the chat and press **Start**.
3. Send `/newbot`.
4. BotFather asks for a **name**, the display name at the top of the chat.
   Anything you like: `My House Camera`.
5. BotFather asks for a **username**. It must be unique across Telegram and end
   in `bot`. If your first choice is taken it says so and asks again; something
   like `smith_house_camera_bot` usually works.

BotFather's reply contains a line like:

```
Use this token to access the HTTP API:
123456789:YOUR_BOT_TOKEN_HERE
```

That long string is your **bot token**. Copy it.

## Step 2: Start a chat with your new bot

Your bot cannot message you until you have messaged it first. This is a
Telegram rule, not an app limitation.

1. In BotFather's reply, tap the `t.me/your_bot_name` link, or search for your
   bot's username.
2. Press **Start**.
3. Send it any message. `hello` will do.

Do not skip this: without it the next step returns nothing and alerts never
arrive.

## Step 3: Connect in SpareEye

Open Setup, paste the bot token, and press **Connect Telegram**. SpareEye finds
the conversation from the message you sent in Step 2, saves its internal ID on
this computer, and sends a confirmation back. You do not need to look up or
copy a chat ID.

Use a new bot that only you have messaged. If more than one conversation has
contacted it, SpareEye refuses to guess where private camera footage should go;
create a fresh bot and connect that one instead.

The desktop setup is now complete.

## Manual chat ID setup for the command-line tool

The headless command-line tool still needs a token and chat ID because it has no
setup screen. Either method below finds the ID.

Either way below works.

### The easy way: @userinfobot

1. Search Telegram for **@userinfobot** and open it.
2. Press **Start**.

It replies with your account details, including an `Id` such as `123456789`.
That is your **chat ID**.

This works because in a one-to-one chat with a bot, your chat ID is the same as
your user ID.

### The direct way: ask Telegram

To avoid a third-party bot, ask Telegram's API directly. Open this URL in a
browser with your own token:

```
https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates
```

Keep the word `bot` immediately before your token, with no space or slash
between them, as in `.../bot8123456789:AAF3.../getUpdates`.

In the JSON that comes back, look for the `"chat"` section:

```json
{"ok":true,"result":[{"update_id":1,"message":{"chat":{"id":123456789,"type":"private"}}}]}
```

The number after `"id":` inside `"chat"` is your chat ID. An empty result,
`{"ok":true,"result":[]}`, means you have not sent your bot a message yet. Go
back to Step 2.

A chat ID can be negative, for example `-1001234567890` for a group. Keep the
minus sign; it is part of the value.

Set both values in the environment before
running:

```
export TELEGRAM_BOT_TOKEN=123456789:YOUR_BOT_TOKEN_HERE
export TELEGRAM_CHAT_ID=123456789
go run ./apps/cli
```

On Windows PowerShell:

```
$env:TELEGRAM_BOT_TOKEN = "123456789:YOUR_BOT_TOKEN_HERE"
$env:TELEGRAM_CHAT_ID = "123456789"
go run ./apps/cli
```

Once the credentials check out, the tool prints `Telegram connected` and sends
a confirmation message. If they do not work, it says so and keeps running with
alerts disabled rather than exiting.

Values saved in `config.json` take precedence; the environment is consulted
only when the config file has no credentials.

## Your bot token is a credential

Treat the bot token like a password.

Anyone holding it can act as your bot, reading messages sent to it and sending
messages through it. For this app, that means asking your camera to record and
having the clip delivered to them.

The app defends against this in one specific way: **every inbound message is
checked against your configured chat ID, and anything from another chat is
discarded before it reaches the detector.** Someone with your token who
messages your bot from their own account gets no response and cannot command
your camera.

That check protects the camera but does not undo everything a leaked token
allows: the holder could still call Telegram's API directly and read messages
your bot has received. So:

- Do not commit the token to a git repository. `.env` and `config.json` are
  both in `.gitignore` for this reason.
- Do not paste it into a chat, an issue, or a screenshot.
- If it does leak, send `/revoke` to @BotFather and pick your bot. It issues a
  new token and the old one stops working immediately. Then update the token in
  the app.

`config.json` is written with owner-only file permissions because it holds this
token.

## Troubleshooting

**`getUpdates` returns `{"ok":true,"result":[]}`**

You have not messaged the bot, or you already consumed the updates. Send a
fresh message and reload the URL. The running detector polls for updates too,
and Telegram hands each update out once, so stop the detector before checking
`getUpdates` by hand.

**"bot token rejected by Telegram" / 401 Unauthorized**

The token is wrong, missing a character, or revoked. The most common cause is a
partial copy: the token contains a colon and a hyphen, both easy to clip. Copy
the whole line after `Use this token to access the HTTP API:`.

**The token looks right but requests still fail**

Check for invisible whitespace. A `.env` file written on Windows ends every
line with a carriage return, which does not show in an editor but becomes part
of the value and makes the request URL invalid. This is why the app trims
credentials before use. If you test the URL by hand in a browser, retype the
token rather than pasting it from a file.

**Alerts never arrive, but the app says it is connected**

The chat ID is wrong, most likely belonging to a different chat than the one
you are watching. Verify it with @userinfobot, and check you have not dropped a
leading minus sign on a group ID.

**Commands sent to the bot are ignored**

Messages from a chat other than the configured one are discarded on purpose, so
messaging the bot from a different Telegram account, or from a group rather
than the direct chat you configured, explains this. Confirm the chat ID matches
the chat you are typing in.

Also check the spelling: the recognised commands are `/record`, `/stop`, and
`/get` (`/list` also works). Anything else is ignored silently.

**A group chat instead of a direct chat**

You can send alerts to a group. Add your bot to the group, send a message
there, and read the chat ID from `getUpdates`; it will be a large negative
number. Everyone in that group receives your camera footage, so be deliberate.

---

Back to the [README](../README.md).
