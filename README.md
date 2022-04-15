# What this is
This is a tool to help you make sense of your trades on [Binance](https://binance.com). <br>
It uses your api key to collect your Binance trade history and report your average buy and sell prices for each token.<br>
Price information is fetched from [Coingecko](https://coingecko.com)<br>
Not all information is captured yet. 

[Demo](https://binalysis.enzosv.xyz)

## Preview
![preview](https://github.com/enzosv/binalysis/blob/main/screenshot.png)

## Features
* Automatic. Only requires Binance API Key and Secret.
* Report is saved as json for faster fetching next time
* Report can be deleted
* No tracking or data collection whatsoever
* Reports all prices in USD

## Limitations
* Does not read balance in Locked Staking
* Only uses string matching for comparing Binance and Coingecko tokens. May be inaccurate.

## Disclaimers
* Not financial advice
* Limited and untested security
* **Make sure to use a read only api key**
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
### Build backend
```
go get -d
go build
```
### Build frontend
```
cd web
go get -d
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o web.wasm
gzip -9 -v -c web.wasm > web.wasm.gz
```
### Run
  1. `./binalysis`
  2. open browser at http://localhost:8080

Tips are appreciated. 0xBa2306a4e2AadF2C3A6084f88045EBed0E842bF9