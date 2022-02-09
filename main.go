package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/binance-exchange/go-binance"
	"github.com/gorilla/mux"
)

type BinanceAuth struct {
	Key string `json:"api_key"`
}
type Asset struct {
	Balance       float64        `json:"balance"`
	BuyQty        float64        `json:"buy_qty"`
	Cost          float64        `json:"cost"`
	SellQty       float64        `json:"sell_qty"`
	Revenue       float64        `json:"revenue"`
	EarliestTrade *binance.Trade `json:"earliest_trade"`
	LatestTrade   *binance.Trade `json:"latest_trade"`
}

var STABLECOINS = map[string]bool{
	"USDT": true,
	"BUSD": true,
	"USDC": true,
	"UST":  true,
}

func (a Asset) compute(trades []*binance.Trade) Asset {
	earliest := a.EarliestTrade
	latest := a.LatestTrade
	for _, t := range trades {
		if t.IsBuyer {
			a.BuyQty += t.Qty
			a.Cost += t.Price * t.Qty
		}
		if !t.IsBuyer {
			a.SellQty += t.Qty
			a.Revenue += t.Price * t.Qty
		}
		if earliest == nil {
			earliest = t
		}
		if latest == nil {
			latest = t
		}
		if earliest.Time.Unix() > t.Time.Unix() {
			earliest = t
		}
		if latest.Time.Unix() < t.Time.Unix() {
			latest = t
		}
	}
	a.EarliestTrade = earliest
	a.LatestTrade = latest
	return a
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "pong")
	})
	r.HandleFunc("/latest", LatestHandler).Methods("POST")
	r.HandleFunc("/update", UpdateHandler).Methods("POST")
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/")))
	srv := &http.Server{
		Handler:      r,
		Addr:         "0.0.0.0:8080",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

func LatestHandler(w http.ResponseWriter, r *http.Request) {
	var a BinanceAuth
	err := json.NewDecoder(r.Body).Decode(&a)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// no extra auth. anyone with key can fetch
	bals := loadExisting(a.Key + ".json")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bals)
}

func UpdateHandler(w http.ResponseWriter, r *http.Request) {
	var a BinanceAuth
	err := json.NewDecoder(r.Body).Decode(&a)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// This is not secure
	secret := r.Header.Get("X-Secret-Key")
	go func(key, secret string) {
		update(context.Background(), a.Key, secret)
		if err != nil {
			fmt.Println(err)
			return
		}
	}(a.Key, secret)
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, "This will take time. Check back later")
}

func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	var a BinanceAuth
	err := json.NewDecoder(r.Body).Decode(&a)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// no extra auth. anyone with key can delete
	err = os.Remove(a.Key + ".json")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "Deleted")
}

func update(ctx context.Context, key, secret string) (map[string]Asset, error) {
	hmacSigner := &binance.HmacSigner{
		Key: []byte(secret),
	}
	binanceService := binance.NewAPIService(
		"https://www.binance.com",
		key,
		hmacSigner,
		nil,
		ctx,
	)
	b := binance.NewBinance(binanceService)

	account, err := b.Account(binance.AccountRequest{
		RecvWindow: 5 * time.Second,
		Timestamp:  time.Now(),
	})
	if err != nil {
		return nil, err
	}
	bals := loadExisting(key + ".json")
	// reset balance
	for k, bal := range bals {
		new := bal
		new.Balance = 0
		bals[k] = new
	}
	for _, bal := range account.Balances {
		value := bal.Free + bal.Locked
		if value < 0.001 {
			continue
		}
		symbol := strings.TrimPrefix(bal.Asset, "LD")
		if _, ok := STABLECOINS[symbol]; ok {
			continue
		}
		var new Asset
		if existing, ok := bals[symbol]; ok {
			new = existing
			new.Balance += value
		} else {
			new = Asset{}
			new.Balance = value
		}

		bals[symbol] = new
	}
	var weight int = 10
	var total int = 0
	// bals = map[string]Asset{}
	// bals["BTC"] = Asset{0, 0, 0, 0, 0, nil, nil}
	for k, existing := range bals {
		var trades []*binance.Trade
		for c := range STABLECOINS {
			var fromID int64 = 0
			// TODO: separate into latest usdt and latest busd
			if existing.LatestTrade != nil {
				fromID = existing.LatestTrade.ID
			}

			for {
				if weight >= 1200 {
					fmt.Println("waiting for limit to refresh")
					time.Sleep(time.Minute)
					weight = 0
				}
				ts, err := b.MyTrades(binance.MyTradesRequest{
					Symbol:     k + c,
					RecvWindow: 5 * time.Second,
					Timestamp:  time.Now(),
					FromID:     fromID,
				})
				weight += 10
				if err != nil {
					fmt.Println(k+c, err)
					break
				}
				if len(ts) < 1 {
					break
				}

				total += len(ts)
				log.Printf("Fetched %d trades\tWeight: %d", total, weight)
				trades = append(trades, ts...)
				fromID = ts[len(ts)-1].ID + 1
			}
		}
		new := existing.compute(trades)
		if new.Cost <= 0 {
			delete(bals, k)
			// never purchased. ignore
			continue
		}
		bals[k] = new
	}
	// for k, v := range bals {
	// 	fmt.Printf("%s: %s to %s\n\tAverage buy: %.2f\n\tAverage sell: %.2f\n",
	// 		k, v.EarliestTrade.Time.Format("2006-01-02"), v.LatestTrade.Time.Format("2006-01-02"),
	// 		v.Cost/v.BuyQty,
	// 		v.Revenue/v.SellQty)
	// }
	file, err := json.Marshal(bals)
	if err != nil {
		return bals, err
	}
	err = ioutil.WriteFile(key+".json", file, 0644)
	return bals, err
}

func loadExisting(path string) map[string]Asset {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return map[string]Asset{}
	}
	var payload map[string]Asset
	json.Unmarshal(content, &payload)
	return payload
}
