package main

import (
	"context"
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

type Payload struct {
	LastUpdate time.Time        `json:"last_update"`
	Assets     map[string]Asset `json:"binance"`
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
	store := flag.String("s", ".", "Directory for storing json. Relative to home")
	flag.Parse()
	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "binalysis pong")
	})
	r.HandleFunc("/latest", LatestHandler(*store)).Methods("GET")
	r.HandleFunc("/update", UpdateHandler(*store)).Methods("POST")
	r.HandleFunc("/del", DeleteHandler(*store)).Methods("DELETE")
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/")))
	fmt.Printf("running at %d\nstoring at %s\n", *port, *store)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), r))
}

func LatestHandler(store string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		key := r.Header.Get("X-API-Key")

		// no extra auth. anyone with key can fetch
		w.Header().Set("Content-Type", "application/json")
		fmt.Printf("%s/%s.json\n", store, key)
		http.ServeFile(w, r, fmt.Sprintf("%s/%s.json", store, key))
	}
}

func UpdateHandler(store string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		// This is not secure
		key := r.Header.Get("X-API-Key")
		path := fmt.Sprintf("%s/%s.json", store, key)
		existing := loadExisting(path)
		if existing.LastUpdate.Unix() > time.Now().Add(-time.Hour).Unix() {
			response := map[string]string{"error": "Updated recently. Try again later"}
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(response)
			return
		}

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
		// create payload with nil trades
		payload, err := fetchBalances(b, existing)
		if err != nil {
			response := map[string]string{"error": err.Error()}
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		file, err := json.Marshal(payload)
		if err != nil {
			response := map[string]string{"error": err.Error()}
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		// save payload
		err = ioutil.WriteFile(path, file, 0644)
		if err != nil {
			response := map[string]string{"error": err.Error()}
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		go func(path string) {
			_, err := update(r.Context(), b, payload.Assets, path)
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println(r.RemoteAddr + " done")
		}(path)
		http.ServeFile(w, r, path)
	}
}

func DeleteHandler(store string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		// no extra auth. anyone with key can delete
		err := os.Remove(fmt.Sprintf("%s/%s.json", store, key))
		if err != nil {
			fmt.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response := map[string]bool{"deleted": true}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
	}
}

func fetchBalances(b binance.Binance, existing Payload) (Payload, error) {
	account, err := b.Account(binance.AccountRequest{
		RecvWindow: 60 * time.Second,
		Timestamp:  time.Now(),
	})
	payload := Payload{existing.LastUpdate, map[string]Asset{}}
	if err != nil {
		return payload, err
	}

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
		if existing_asset, ok := payload.Assets[symbol]; ok {
			new = existing_asset
			new.Balance += value
		} else {
			new = Asset{}
			new.Balance = value
		}

		payload.Assets[symbol] = new
	}
	return payload, nil
}

func update(ctx context.Context, b binance.Binance, balances map[string]Asset, path string) (map[string]Asset, error) {
	last_update := time.Now()
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
	payload := Payload{last_update, bals}
	file, err := json.Marshal(payload)
	if err != nil {
		return bals, err
	}

	err = ioutil.WriteFile(path, file, 0644)
	return bals, err
}

func loadExisting(path string) Payload {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return Payload{time.Time{}, map[string]Asset{}}
	}
	var payload Payload
	json.Unmarshal(content, &payload)
	return payload
}
