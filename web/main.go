package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"syscall/js"
	"time"
)

type Payload struct {
	LastUpdate time.Time        `json:"last_update"`
	Binance    map[string]Asset `json:"binance"`
}
type Asset struct {
	Balance float64         `json:"balance"`
	Pairs   map[string]Pair `json:"pairs"`
}
type Pair struct {
	BuyQty        float64 `json:"buy_qty"`
	Cost          float64 `json:"cost"`
	SellQty       float64 `json:"sell_qty"`
	Revenue       float64 `json:"revenue"`
	EarliestTrade *Trade  `json:"earliest_trade"`
	LatestTrade   *Trade  `json:"latest_trade"`
}
type Coin struct {
	ID        string  `json:"id"`
	Symbol    string  `json:"symbol"`
	Name      string  `json:"name"`
	USD       float64 `json:"usd"`
	MarketCap float64 `json:"usd_market_cap"`
	Change    float64 `json:"usd_24h_change"`
}

// from binance-go
type Trade struct {
	ID              int64
	Price           float64
	Qty             float64
	Commission      float64
	CommissionAsset string
	Time            time.Time
	IsBuyer         bool
	IsMaker         bool
	IsBestMatch     bool
}

func main() {
	js.Global().Set("gorefresh", refreshWrapper())
	select {}
}

func refreshWrapper() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		key := args[0].String()
		url := args[1].String()
		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) != 2 {
				return "key and url are required"
			}
			resolve := args[0]
			reject := args[1]
			go func(key, url string) {
				output, err := refresh(key, url)
				if err != nil {
					reject.Invoke(err.Error())
					return
				}
				resolve.Invoke(output)
			}(key, url)

			return nil
		})
		promiseConstructor := js.Global().Get("Promise")
		return promiseConstructor.New(handler)
	})
}

func refresh(key, url string) (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	payloadChan := make(chan Payload)
	coinlistChan := make(chan []Coin)
	errorChan := make(chan error)
	go func(ch chan Payload, ech chan error) {
		payload, err := fetchLatest(client, key, url)
		if err != nil {
			ech <- err
			return
		}
		ch <- payload
	}(payloadChan, errorChan)
	go func(ch chan []Coin, ech chan error) {
		coinlist, err := fetchCoinList(client)
		if err != nil {
			ech <- err
			return
		}
		ch <- coinlist
	}(coinlistChan, errorChan)

	var payload Payload
	var coinlist []Coin
	select {
	case err := <-errorChan:
		return "", err
	case p := <-payloadChan:
		payload = p
	}

	select {
	case err := <-errorChan:
		return "", err
	case cl := <-coinlistChan:
		coinlist = cl
	}
	coins, err := matchCoins(client, payload, coinlist)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	cleaned := usdOnly(payload, coins)
	output, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return string(output), nil
}

func fetchLatest(client *http.Client, key, url string) (Payload, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Payload{}, err
	}

	req.Header.Add("X-API-Key", key)
	req.Header.Add("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return Payload{}, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return Payload{}, err
	}
	defer res.Body.Close()
	var payload Payload
	err = json.Unmarshal(body, &payload)
	if err != nil {
		return Payload{}, err
	}
	return payload, nil
}

func fetchCoinList(client *http.Client) ([]Coin, error) {
	req, err := http.NewRequest("GET", "https://api.coingecko.com/api/v3/coins/list", nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var coinlist []Coin
	err = json.Unmarshal(body, &coinlist)
	if err != nil {
		return nil, err
	}
	return coinlist, nil
}

func matchCoins(client *http.Client, payload Payload, coinlist []Coin) (map[string]Coin, error) {
	var coinids []string
	coins := map[string]Coin{}
	for symbol, asset := range payload.Binance {
		if len(asset.Pairs) < 1 {
			continue
		}
		s := strings.ToLower(symbol)
		for _, coin := range coinlist {
			// ignore if duplicate. is that even possible?
			token := strings.ToLower(coin.Symbol)
			if strings.Contains(strings.ToLower(coin.ID), "wormhole") {
				// it's never this
				continue
			}
			// TODO: handle IOTA in binance vs miota in coingecko
			if s == token {
				coinids = append(coinids, coin.ID)
				coins[token] = Coin{}
				continue
			}
			for k := range asset.Pairs {
				if token != strings.ToLower(k) {
					continue
				}
				coinids = append(coinids, coin.ID)
				coins[token] = Coin{}
			}
		}
	}

	req, err := http.NewRequest("GET", "https://api.coingecko.com/api/v3/simple/price", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Add("ids", strings.Join(coinids, ","))
	q.Add("vs_currencies", "usd")
	q.Add("include_24hr_change", "true")
	q.Add("include_market_cap", "true")
	req.URL.RawQuery = q.Encode()
	fmt.Println(req.URL)
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var data map[string]map[string]float64
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	for k, v := range data {
		marketCap := v["usd_market_cap"]
		for _, coin := range coinlist {
			if coin.ID != k {
				continue
			}
			symbol := strings.ToLower(coin.Symbol)
			new := coins[symbol]
			if new.MarketCap >= marketCap {
				break
			}
			new.ID = k
			new.Name = coin.Name
			new.Symbol = coin.Symbol
			new.MarketCap = marketCap
			new.Change = v["usd_24h_change"]
			new.USD = v["usd"]
			coins[symbol] = new
			break
		}
	}
	return coins, nil
}

type Clean struct {
	Coin          Coin `json:"coin"`
	AverageBuy    float64
	AverageSell   float64
	Cost          float64
	Revenue       float64
	BuyQty        float64
	SellQty       float64
	EarliestTrade Trade
	LatestTrade   Trade
	Balance       float64
	Profit        float64
	Dif           float64
	PercentDif    float64
}

func usdOnly(payload Payload, coins map[string]Coin) map[string]Clean {
	stablecoins := map[string]bool{
		"usdt": true,
		"busd": true,
		"usdc": true,
		"tusd": true,
	}
	cleaned := map[string]Clean{}
	for k, v := range payload.Binance {
		if len(v.Pairs) < 1 {
			continue
		}
		if _, ok := coins[strings.ToLower(k)]; !ok {
			continue
		}
		clean := Clean{}
		clean.EarliestTrade.Time = time.Unix(9223372036854775807, 0)
		clean.LatestTrade.Time = time.Unix(0, 0)
		clean.Coin = coins[strings.ToLower(k)]
		clean.Balance = v.Balance
		for kk, vv := range v.Pairs {
			new := vv
			symbol := strings.ToLower(kk)
			if _, ok := stablecoins[symbol]; !ok {

				coin := coins[symbol]
				fmt.Println(symbol, coin)
				new.Cost *= coin.USD
				new.Revenue *= coin.USD
				new.EarliestTrade.Price *= coin.USD
				new.LatestTrade.Price *= coin.USD
			}

			clean.BuyQty += new.BuyQty
			clean.Cost += new.Cost
			clean.SellQty += new.SellQty
			clean.Revenue += new.Revenue
			if clean.EarliestTrade.Time.Unix() > new.EarliestTrade.Time.Unix() {
				clean.EarliestTrade = *new.EarliestTrade
			}
			if clean.LatestTrade.Time.Unix() < new.LatestTrade.Time.Unix() {
				clean.LatestTrade = *new.LatestTrade
			}
		}

		if clean.BuyQty != 0 {
			clean.AverageBuy = clean.Cost / clean.BuyQty
			clean.Dif = clean.Coin.USD - clean.AverageBuy
			divisor := (clean.Coin.USD + clean.AverageBuy) / 2
			if divisor != 0 {
				clean.PercentDif = clean.Dif * 100 / divisor
			}
		}
		if clean.SellQty != 0 {
			clean.AverageSell = clean.Revenue / clean.SellQty
		}

		clean.Profit = clean.Revenue - clean.Cost + clean.Balance*clean.Coin.USD

		_, err := json.MarshalIndent(clean, "", "  ")
		if err != nil {
			fmt.Println(k, clean, err)
			continue
		}
		cleaned[k] = clean
	}
	return cleaned
}
