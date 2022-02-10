# What this is
This is a tool to help you make sense of your trades on [binance](https://binance.com). <br>
It uses your api key to collect your binance trade history and report your average buy and sell prices for each token.<br>
Price information is fetched from [coingecko](https://coingecko.com)<br>
Not all information is captured yet. 

[Demo](https://binalysis.enzosv.xyz)

## Preview
![preview](https://github.com/enzosv/binalysis/blob/main/screenshot.png)

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
* Limited and untested security
* **Make sure to use a read only api key**
* Price matching may be inaccurate
* Work in progress

## TODO
* Better website
* Capture more data
* Support more exchanges
* Notifications for finished update
* Notifications for price above/below average buy

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
open browser at http://localhost:8080

Tips are appreciated. 0xBa2306a4e2AadF2C3A6084f88045EBed0E842bF9