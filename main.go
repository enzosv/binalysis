package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/binance-exchange/go-binance"
	"github.com/gorilla/mux"
)

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
	"USDC": false, // do not fetch pairs against this
	"UST":  false,
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
	port := flag.Int("p", 8080, "port to use")
	store := flag.String("s", "/binalysis", "Directory for storing json. Relative to home")
	flag.Parse()
	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "binalysis pong")
	})
	r.HandleFunc("/latest", LatestHandler(*store)).Methods("GET")
	r.HandleFunc("/update", UpdateHandler(*store)).Methods("POST")
	r.HandleFunc("/del", UpdateHandler(*store)).Methods("DELETE")
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/")))
	fmt.Printf("running at %d\nstoring at %s", *port, *store)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), r))
}

func LatestHandler(store string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		key := r.Header.Get("X-API-Key")
		// no extra auth. anyone with key can fetch
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, fmt.Sprintf("%s/%s.json", store, key))
	}
}

func UpdateHandler(store string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		// This is not secure
		key := r.Header.Get("X-API-Key")
		secret := r.Header.Get("X-Secret-Key")
		hmacSigner := &binance.HmacSigner{
			Key: []byte(secret),
		}
		binanceService := binance.NewAPIService(
			"https://www.binance.com",
			key,
			hmacSigner,
			nil,
			r.Context(),
		)
		b := binance.NewBinance(binanceService)
		balances, err := fetchBalances(b, key)
		if err != nil {
			response := map[string]string{"error": err.Error()}
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		go func(key, store string) {
			_, err := update(b, balances, fmt.Sprintf("%s/%s.json", store, key))
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println(r.RemoteAddr + " done")
		}(key, store)

		json.NewEncoder(w).Encode(balances)
	}
}

func DeleteHandler(store string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		// no extra auth. anyone with key can delete
		err := os.Remove(fmt.Sprintf("%s/%s.json", store, key))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response := map[string]bool{"deleted": true}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
	}
}

func fetchBalances(b binance.Binance, key string) (map[string]Asset, error) {
	account, err := b.Account(binance.AccountRequest{
		RecvWindow: 60 * time.Second,
		Timestamp:  time.Now(),
	})
	if err != nil {
		return nil, err
	}
	bals := loadExisting(key + ".json")
	for _, bal := range account.Balances {
		value := bal.Free + bal.Locked
		if value <= 0 {
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
	return bals, nil
}

func update(b binance.Binance, balances map[string]Asset, path string) (map[string]Asset, error) {
	var weight int = 10 // from fetch balance
	var total int = 0
	// bals = map[string]Asset{}
	// bals["BTC"] = Asset{0, 0, 0, 0, 0, nil, nil}
	bals := balances
	for k, existing := range bals {
		var trades []*binance.Trade
		for c, valid := range STABLECOINS {
			if !valid {
				continue
			}
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
					RecvWindow: 60 * time.Second,
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
	log.Printf("Fetched %d trades", total)
	file, err := json.Marshal(bals)
	if err != nil {
		return bals, err
	}
	err = ioutil.WriteFile(path, file, 0644)
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
