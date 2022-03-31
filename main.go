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
	"strconv"
	"strings"
	"time"

	"github.com/NYTimes/gziphandler"
	binance2 "github.com/adshao/go-binance/v2"
	"github.com/binance-exchange/go-binance"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type PairsResponse struct {
	Data []struct {
		// Product string `json:"s"`
		// Type    string `json:"st"`
		Buying  string `json:"b"`
		Selling string `json:"q"`
	} `json:"data"`
}
type Asset struct {
	Balance                float64         `json:"balance"`
	Pairs                  map[string]Pair `json:"pairs"`
	LatestDistributionTime int64           `json:"latest_distribution_time"`
	DistributionTotal      float64         `json:"distribution_total"`
}

type Pair struct {
	BuyQty        float64            `json:"buy_qty"`
	Cost          float64            `json:"cost"`
	SellQty       float64            `json:"sell_qty"`
	Revenue       float64            `json:"revenue"`
	Fees          map[string]float64 `json:"fees"`
	EarliestTrade *binance.Trade     `json:"earliest_trade"`
	LatestTrade   *binance.Trade     `json:"latest_trade"`
}

type Payload struct {
	LastUpdate time.Time        `json:"last_update"`
	Assets     map[string]Asset `json:"binance"`
}

func (a Asset) compute(selling string, trades []*binance.Trade) Asset {
	pair := Pair{}
	if value, ok := a.Pairs[selling]; ok {
		pair = value
	}
	earliest := pair.EarliestTrade
	latest := pair.LatestTrade
	fees := map[string]float64{}
	for _, t := range trades {
		fees[t.CommissionAsset] += t.Commission
		if t.IsBuyer {
			pair.BuyQty += t.Qty
			pair.Cost += t.Price * t.Qty
		}
		if !t.IsBuyer {
			pair.SellQty += t.Qty
			pair.Revenue += t.Price * t.Qty
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
	pair.EarliestTrade = earliest
	pair.LatestTrade = latest
	pair.Fees = fees
	new := a
	if new.Pairs == nil {
		new.Pairs = map[string]Pair{}
	}
	new.Pairs[selling] = pair
	return new
}

func main() {
	port := flag.Int("p", 8080, "port to use")
	store := flag.String("s", ".", "Directory for storing json. Relative to home")
	verbose := flag.Bool("v", false, "print info logs")
	flag.Parse()
	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "binalysis pong")
	})
	r.HandleFunc("/latest", LatestHandler(*store, *verbose)).Methods("GET")
	r.HandleFunc("/update", UpdateHandler(*store, *verbose)).Methods("POST")
	r.HandleFunc("/del", DeleteHandler(*store, *verbose)).Methods("DELETE")
	r.PathPrefix("/").Handler(gziphandler.GzipHandler(http.FileServer(http.Dir("./web/"))))
	if *verbose {
		fmt.Printf("running at %d\nstoring at %s\n", *port, *store)
	}

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), r))
}

func LatestHandler(store string, verbose bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		key := r.Header.Get("X-API-Key")

		// no extra auth. anyone with key can fetch
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, fmt.Sprintf("%s/%s.json", store, key))
	}
}

func UpdateHandler(store string, verbose bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		// This is not secure
		key := r.Header.Get("X-API-Key")
		path := fmt.Sprintf("%s/%s.json", store, key)
		existing := loadExisting(path)
		nextAvailable := existing.LastUpdate.Add(time.Minute * 1)
		if time.Now().Unix() < nextAvailable.Unix() {
			response := map[string]string{"error": fmt.Sprintf("Updated recently. Try again at %s", nextAvailable.Add(time.Minute).Format("3:04PM"))}
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

		payload, err := fetchBalances(b, existing, verbose)
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
		go func(ctx context.Context, path, key, secret string) {
			start := time.Now().Unix()
			_, err := update(ctx, b, payload, path, verbose)
			if err != nil {
				fmt.Println(err)
				return
			}
			// perform after update to ignore untraded
			client := binance2.NewClient(key, secret)
			for k, v := range payload.Assets {
				latest, total, err := fetchDistributions(ctx, client, k, v.DistributionTotal, v.LatestDistributionTime)
				if err != nil {
					continue
				}
				v.DistributionTotal = total
				v.LatestDistributionTime = latest
				payload.Assets[k] = v
			}
			if verbose {
				fmt.Printf("%s done after %d seconds\n", r.RemoteAddr, time.Now().Unix()-start)
			}
		}(r.Context(), path, key, secret)
		http.ServeFile(w, r, path)
	}
}

func DeleteHandler(store string, verbose bool) http.HandlerFunc {
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

func fetchDistributions(ctx context.Context, client *binance2.Client, symbol string, total float64, start int64) (int64, float64, error) {
	request := client.NewAssetDividendService().Asset(symbol).Limit(500)
	if start > 0 {
		request = request.StartTime(start + 1).EndTime(time.Now().Unix())
	}
	distributions, err := request.Do(ctx)
	if err != nil {
		fmt.Println(err)
		return 0, 0, err
	}
	rows := *distributions.Rows
	newTotal := total
	for _, d := range rows {
		amount, err := strconv.ParseFloat(d.Amount, 64)
		if err != nil {
			fmt.Println(err)
			return 0, 0, err
		}
		newTotal += amount
	}
	if len(rows) >= 500 {
		// TODO: fetch more distributions
		// return fetchDistributions(ctx, client, symbol, newTotal, rows[0].Time+1)
	}
	if len(rows) < 1 {
		return 0, 0, nil
	}
	return rows[0].ID, newTotal, nil
}

func fetchBalances(b binance.Binance, existing Payload, verbose bool) (Payload, error) {
	// TODO: fetch withdraws, desposits, dust conversions, transfers
	// TODO: fetch earn distributions and balances
	// https://www.reddit.com/r/binance/comments/k6b1r7/accessing_earn_with_api/
	// https://www.binance.com/bapi/earn/v1/private/lending/daily/token/position?pageIndex=2&pageSize=20
	// https://www.binance.com/bapi/capital/v1/private/streamer/trade/get-user-trades
	// https://binance-docs.github.io/apidocs/spot/en/#lending-account-user_data
	account, err := b.Account(binance.AccountRequest{
		RecvWindow: 60 * time.Second,
		Timestamp:  time.Now(),
	})
	if err != nil {
		return Payload{time.Now(), existing.Assets}, err
	}

	// zero out balances
	assets := map[string]Asset{}
	for i, bal := range existing.Assets {
		new := bal
		new.Balance = 0
		assets[i] = new
	}

	for _, bal := range account.Balances {

		value := bal.Free + bal.Locked
		// uncomment to ignore assets with no balance
		// if value <= 0 {
		// continue
		// }

		symbol := strings.TrimPrefix(bal.Asset, "LD")
		var new Asset
		if existing_asset, ok := assets[symbol]; ok {
			new = existing_asset
			new.Balance = value
		} else {
			new = Asset{}
			new.Balance = value
			if verbose {
				fmt.Println("new asset", symbol)
			}
		}
		assets[symbol] = new
	}

	return Payload{time.Now(), assets}, nil
}

func fetchPairs() (PairsResponse, error) {
	var pairs PairsResponse
	res, err := http.Get("https://www.binance.com/bapi/asset/v2/public/asset-service/product/get-products")
	if err != nil {
		return pairs, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return pairs, errors.Wrap(err, "unable to read response from get-products")
	}
	defer res.Body.Close()

	if err := json.Unmarshal(body, &pairs); err != nil {
		return pairs, errors.Wrap(err, "pairs unmarshal failed")
	}
	return pairs, nil
}

func update(ctx context.Context, b binance.Binance, payload Payload, path string, verbose bool) (map[string]Asset, error) {

	pairs, err := fetchPairs()
	if err != nil {
		return nil, err
	}
	bals := payload.Assets
	var total int = 0
	// TODO: prioritize balances that have changed and are > 0
	for k, existing := range bals {
		new := existing
		for _, p := range pairs.Data {
			if p.Buying != k {
				continue
			}
			var trades []*binance.Trade
			product := k + p.Selling
			var fromID int64 = 0
			if value, ok := existing.Pairs[p.Selling]; ok && value.LatestTrade != nil {
				// get latest fromID from persisted to save on requests
				// +1 because mytrades is inclusive on fromid
				fromID = value.LatestTrade.ID + 1
			}
			for {
				// keep fetching trades against product until error or < 1 trades returned
				ts, err := b.MyTrades(binance.MyTradesRequest{
					Symbol:     product,
					RecvWindow: 60 * time.Second,
					Timestamp:  time.Now(),
					FromID:     fromID,
				})
				if err != nil {
					if strings.HasPrefix(err.Error(), "-1003") {
						// api rate limit. Wait
						// persist while waiting
						// TODO: do not persist if nothing new since last persist

						go func(bals map[string]Asset, path string, total int, verbose bool) {
							// ok to ignore persist error. It will be retried
							// persist despite nothing new to update last_update
							persist(bals, path, total, verbose)

						}(bals, path, total, verbose)
						if verbose {
							fmt.Printf("[%s] Waiting for limit to refresh\n", product)
						}
						time.Sleep(time.Minute)
						// fromID not updated so it will be retried on continue
						continue
					} else {
						err = errors.Wrap(err, fmt.Sprintf("[%s] fetching", product))
						fmt.Println(err)
						break
					}
				}
				if len(ts) < 1 {
					// no more trades for this product
					break
				}
				if verbose {
					fmt.Printf("[%s] fetched %d trades starting from id %d\n", product, len(ts), fromID)
				}

				total += len(ts)
				trades = append(trades, ts...)
				// because mytrades is inclusive on fromid
				fromID = ts[len(ts)-1].ID + 1
			}
			// update asset with fetched product trades
			// ignore products with no trades
			if len(trades) > 0 {
				new = new.compute(p.Selling, trades)
			}
		}
		// update asset map after fetching all trades for a product
		if new.Pairs == nil {
			// remove untraded
			if verbose {
				fmt.Printf("%s untraded. Removing\n", k)
			}
			delete(bals, k)
		} else {
			bals[k] = new
		}

	}
	if verbose {
		fmt.Printf("Fetched %d new trades since %s\n", total, payload.LastUpdate.Format("2006-01-02 3:04PM"))
	}
	// persist despite nothing new to update last_update
	err = persist(bals, path, total, verbose)
	return bals, err
}

func persist(assets map[string]Asset, path string, total int, verbose bool) error {
	payload := Payload{time.Now(), assets}
	file, err := json.Marshal(payload)
	if err != nil {
		err = errors.Wrap(err, "encoding")
		return err
	}
	// TODO: encrypt
	err = ioutil.WriteFile(path, file, 0644)
	if err != nil {
		err = errors.Wrap(err, "persisting")
		return err
	}
	if verbose {
		fmt.Printf("%d trades saved\n", total)
	}
	return nil
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
