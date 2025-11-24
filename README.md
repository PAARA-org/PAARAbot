# PAARAbot

PAARAbot is a Discord bot written in GoLang (<golang.org>) using the DiscordGo library (<github.com/bwmarrin/discordgo>).

On startup, the bot reads from a list of ham callsigns (Club members, or Discord server members), and then periodically retrieves the list of active SOTA and POTA spots. If it finds any spots for the tracked callsigns, it will post a Discord message containing the `Call Sign`, `Park or Peak`, `Frequency` and `Mode`.

# Usage

Sample usage:

```bash
$ ./PAARAbot \
  -hamfile=paara_members.txt \
  -sotacsv=sota_pota.csv \
  -postThrottleTime=4.5h \
  -spotCheckInterval=3m \
  -token=abc \
  -potaChannelID=xxx \
  -sotaChannelID=yyy
```

The **paara_members.txt** file is mandatory and must contain at least one callsign. The **sota_pota.csv** file is optional.

## `-token`, `-potaChannelID` and `-sotaChannelID`

These three values are mandatory to allow the bot to connect to Discord and post messages.

To generate a token, you need to visit <https://discord.com/developers/applications> and create a new application. For more information, please check this [Medium](https://medium.com/@mssandeepkamath/building-a-simple-discord-bot-using-go-12bfca31ad5d) article, or search `how to generate a discord bot token` on <Google.com>.

The `-potaChannelID` and `-sotaChannelID` can be set to different values, or to the same value. Our Club's (<paara.org>) Discord server had initially posted them separately in either the `#pota` or `#sota` channel, but we later converged into one channel named `#spots`.

If you want to use a single channel, use the same channelID for both variables.

## `-hamfile`

This flag sets the filename containing the list of interesting ham call signs.

The format of the file is very simple: one call sign per line. The parser will ignore any empty or commented lines (starting with `#` or `//`).

An example file is provided in `examples/callsigns_sample.txt`:

```
# This is a sample file containing callsigns that must match this format:
# * the commented files are ignored
# * one callsign per line
#
KN6YUH
AK6EU
```

## `-spotCheckInterval`

This flag controls the interval for checking the POTA and SOTA for new spots. The default is 2 minutes, and I'd recommend not setting is to something shorter than this, to avoid getting blocked for refreshing the page too often.

## `-postThrottleTime`

This flag controls how often the same combination of CallSign and POTA/SOTA entity will cause a new message to be posted to Discord.

It's highly recommended to not reduce this flag to less than 1 hour, as that could cause doubling the posts.

## `-help`

```bash
% ./PAARAbot --helpshort
flag provided but not defined: -helpshort
Usage of ./PAARAbot:
  -hamfile string
    	File containing the list of ham callsigns to check for activations.
  -postThrottleTime duration
    	How often to re-post the same spot. (default 4h0m0s)
  -potaChannelID string
    	POTA channel ID from Discord.
  -sotaChannelID string
    	SOTA channel ID from Discord.
  -sotacsv string
    	CSV file containing mapping from peak to park.
  -spotCheckInterval duration
    	How often to check for new spots (default 2m0s)
  -token string
    	Discord bot token
  -version
    	Display application build information and exit.
```


# SOTA activations in POTA parks

This is a feature requested by **Gabriel** [AJ6X](https://www.qrz.com/db/AJ6X) on 06/17/2025. When a SOTA activation is detected by the bot, it will check whether the peak is located in a POTA location and, if true, will post an additional message in the `#pota` Discord channel.

The mapping from PEAK to PARK is done by parsing this CSV: https://raw.githubusercontent.com/aj6x/sota/refs/heads/main/data/sota_pota.csv

You will need to fetch a copy of this CSV file locally and point the bot at it using the `-sotacsv` flag.

# Credits

This is a Discord bot initially based on the example provided at https://medium.com/@mssandeepkamath/building-a-simple-discord-bot-using-go-12bfca31ad5d.
