# What this is
This is a tool to help you make sense of your trades on [binance](https://binance.com). 
It uses your api key to summarize your trades and report your average buy and sell prices for each token.
Price information is fetched from [coingecko](https://coingecko.com)
Not all information is captured yet. 


## Features
* Report is saved as json for faster fetching next time
* Report can be deleted
* No tracking or data collection whatsoever

## Limitations
* Only fetches trades for crypto with balance above 0
* Trades for USDT, BUSD, USDC, UST are ignored
* Only matches crypto pairs against USDC and BUSD

## Disclaimers
* Not financial advice
* Make sure to use a read only api key
* Price matching may be inaccurate
* Work in progress

## TODO
* Better website
* Capture more data
* Support more exchanges

# How to
## Requirements
1. go
2. [Binance API Key](https://www.binance.com/en/support/faq/360002502072/)
## Steps
```
go get -d
go build
./binalysis
```

Tips are appreciated. 0xBa2306a4e2AadF2C3A6084f88045EBed0E842bF9