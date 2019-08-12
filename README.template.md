# ${APP}

Emoji reactions bot for Telegram. Public instance: [@emoji_reactions_bot](https://t.me/emoji_reactions_bot)

![screenshot](docs/shot.png)


## Contents

- [Set-up](#set-up)
- [Get it](#get-it)
- [Usage](#usage)

## Set-up

1. Add the bot [@emoji_reactions_bot](https://t.me/emoji_reactions_bot) (or your own instance) to a group
2. Give it admin (`Delete messages`) rights
3. See [Usage](#usage)

## Get it

Using go get:

```bash
go get -u github.com/sgreben/${APP}
```

Or [download the binary for your platform](https://github.com/sgreben/${APP}/releases/latest) from the releases page.

## Usage

### Telegram

1. Reply to a message with only emoji
2. Add further emoji by either...
   - ...using the buttons to `+1` existing emoji, or
   - ...replying to the button post with new emoji

### CLI

```text
${APP} -token BOT_TOKEN

${USAGE}
```
