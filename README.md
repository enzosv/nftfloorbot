# What this is
Telegram bot to notify you of changing floor prices for NFT collections you choose to watch.
## Features
* Simple setup and forget. No database or server configuration necessary.
* Configurable with other secondary marketplaces.
## Limitations
* Only tested and configured to work with [Opensea](https://opensea.io/) and [MagicEden](https://www.magiceden.io/) so far
* You need to add find the slugs for the collections you are watching and add to config manually
## Preview
![preview](https://github.com/enzosv/nftfloorbot/blob/main/screenshot.png)
## Notes
This gets spammy so I recommend to only run it when you're actively looking to buy

# Build and run
## Requirements
* go
* config.json. See [sample_config.json](https://github.com/enzosv/nftfloorbot/blob/main/sample_config.json) for more details.
## Steps
```
go get -d
go build
./floorbot
```
or
```
go run main.go
```

