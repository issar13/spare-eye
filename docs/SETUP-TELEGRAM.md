# Setting up Telegram

Kaa Rada sends alerts through a Telegram bot that **you** create. There
is no shared bot and no token built into the app. Your footage goes to your bot
and your chat, and nobody else's.

You need two pieces of information:

1. A **bot token** — identifies the bot the app sends messages as
2. A **chat ID** — identifies the conversation those messages go to

This takes about five minutes. You need the Telegram app on your phone or
desktop; nothing else.

## Step 1 — Create the bot

1. Open Telegram and search for **@BotFather**. It is Telegram's official bot
   for making bots, and it has a blue verified checkmark next to its name.
2. Open the chat and press **Start**.
3. Send `/newbot`.
4. BotFather asks for a **name**. This is the display name shown at the top of
   the chat. Anything you like — `My House Camera`.
5. BotFather asks for a **username**. This must be unique across all of
   Telegram and must end in `bot`. If your first choice is taken it will say so
   and ask again. Something like `smith_house_camera_bot` usually works.

BotFather replies with a congratulations message containing a line like:

```
Use this token to access the HTTP API:
123456789:YOUR_BOT_TOKEN_HERE
```

That long string is your **bot token**. Copy it.

## Step 2 — Start a chat with your new bot

Your bot cannot message you until you have messaged it first. This is a
Telegram rule, not an app limitation.

1. In BotFather's reply, tap the `t.me/your_bot_name` link, or search for your
   bot's username.
2. Press **Start**.
3. Send it any message — `hello` will do.

Do not skip this. If you do, the next step returns nothing and alerts will
never arrive.

## Step 3 — Find your chat ID

Two ways. Either is fine.

### The easy way: @userinfobot

1. Search Telegram for **@userinfobot** and open it.
2. Press **Start**.

It replies immediately with your account details, including an `Id` — a number
like `123456789`. That is your **chat ID**.

This works because for a one-to-one chat with a bot, your chat ID is the same
as your user ID.

### The direct way: ask Telegram

If you would rather not involve a third-party bot, ask Telegram's API directly.
Open this URL in a browser, substituting your own token:

```
https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates
```

Keep the word `bot` immediately before your token — the URL is
`.../bot8123456789:AAF3.../getUpdates`, with no space or slash between them.

You will get back a block of JSON. Look for the `"chat"` section:

```json
{"ok":true,"result":[{"update_id":1,"message":{"chat":{"id":123456789,"type":"private"}}}]}
```

The number after `"id":` inside `"chat"` is your chat ID. If the response is
`{"ok":true,"result":[]}` — an empty result — you have not sent your bot a
message yet. Go back to Step 2.

Note that a chat ID can be negative, for example `-1001234567890` if you are
using a group. Keep the minus sign; it is part of the value.

## Step 4 — Enter both values

**In the desktop app** (once available): open the setup screen, paste the bot
token and chat ID, and use *Test connection*. A successful test sends a
confirmation message to your chat, so you know delivery works before you rely
on it.

**With the headless command-line tool**, set both in the environment before
running:

```
export TELEGRAM_BOT_TOKEN=123456789:YOUR_BOT_TOKEN_HERE
export TELEGRAM_CHAT_ID=123456789
go run ./cmd/kaa-rada-cli
```

On Windows PowerShell:

```
$env:TELEGRAM_BOT_TOKEN = "123456789:YOUR_BOT_TOKEN_HERE"
$env:TELEGRAM_CHAT_ID = "123456789"
go run ./cmd/kaa-rada-cli
```

The tool prints `Telegram connected` and sends a confirmation message once the
credentials check out. If the credentials do not work, it says so and keeps
running with alerts disabled rather than exiting.

Values saved in `config.json` take precedence; the environment is only consulted
when the config file has no credentials.

## Your bot token is a credential

Treat the bot token the way you would treat a password.

Anyone holding it can act as your bot. They can read the messages sent to it and
send messages through it, which for this app means they could ask your camera to
start recording and have the clip delivered to them.

The app defends against this in one specific way: **every inbound message is
checked against your configured chat ID, and anything from another chat is
discarded before it reaches the detector.** Someone with your token who messages
your bot from their own account gets no response and cannot command your camera.

That check protects the camera. It does not undo everything a leaked token
allows — someone holding it could still call Telegram's API directly and read
messages your bot has received. So:

- Do not commit the token to a git repository. `.env` and `config.json` are both
  in `.gitignore` for this reason.
- Do not paste it into a chat, an issue, or a screenshot.
- If it does leak, send `/revoke` to @BotFather, pick your bot, and it will
  issue a new token. The old one stops working immediately. Then update the
  token in the app.

`config.json` is written with owner-only file permissions because it holds this
token.

## Troubleshooting

**`getUpdates` returns `{"ok":true,"result":[]}`**

You have not messaged the bot, or you already consumed the updates. Send your
bot a fresh message and reload the URL. Note that once the detector is running
it polls for updates too, and Telegram hands each update out once — so stop the
detector before checking `getUpdates` by hand.

**"bot token rejected by Telegram" / 401 Unauthorized**

The token is wrong, has a character missing, or has been revoked. The most
common cause is a partial copy — the token contains a colon and a hyphen, both
of which are easy to clip. Copy the whole line after `Use this token to access
the HTTP API:`.

**The token looks right but requests still fail**

Check for invisible whitespace. A `.env` file written on Windows ends every line
with a carriage return, which does not show in an editor but becomes part of the
value and makes the request URL invalid. This is why the app trims credentials
before use. If you are testing the URL by hand in a browser, retype the token
rather than pasting it from a file.

**Alerts never arrive, but the app says it is connected**

The chat ID is wrong — most likely it belongs to a different chat than the one
you are watching. Verify it with @userinfobot, and check you have not dropped a
leading minus sign on a group ID.

**Commands sent to the bot are ignored**

Messages from a chat other than the configured one are discarded on purpose. If
you are messaging the bot from a different Telegram account, or from a group
rather than the direct chat you configured, that is why. Confirm the chat ID
matches the chat you are typing in.

Also check the spelling: the recognised commands are `/record`, `/stop`, and
`/get` (`/list` also works). Anything else is ignored silently.

**A group chat instead of a direct chat**

You can send alerts to a group. Add your bot to the group, send a message there,
and read the chat ID from `getUpdates` — it will be a large negative number.
Everyone in that group will receive your camera footage, so be deliberate about
it.

---

Back to the [README](../README.md).
