package main

import (
	"context"
	"crypto/rand"
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

	"github.com/Kucoin/kucoin-go-sdk"
	"github.com/NYTimes/gziphandler"
	binance2 "github.com/adshao/go-binance/v2"
	common "github.com/adshao/go-binance/v2/common"
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
	Kucoin     map[string]Asset `json:"kucoin"`
}

func (p Payload) persist(path string) error {
	p.LastUpdate = time.Now()
	file, err := json.Marshal(p)
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
	return nil
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

var hmacSecret []byte

func main() {
	port := flag.Int("p", 8080, "port to use")
	store := flag.String("s", ".", "Directory for storing json. Relative to home")
	verbose := flag.Bool("v", false, "print info logs")
	flag.Parse()

	secret, err := generateRandomBytes(32)
	if err != nil {
		log.Fatal(err)
	}
	hmacSecret = secret

	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "binalysis pong")
	})
	r.HandleFunc("/latest", LatestHandler(*store, *verbose)).Methods("GET")
	r.HandleFunc("/update", UpdateHandler(*store, *verbose)).Methods("POST")
	r.HandleFunc("/del", DeleteHandler(*store, *verbose)).Methods("DELETE")
	r.PathPrefix("/").Handler(gziphandler.GzipHandler(http.FileServer(http.Dir("./web/"))))
	// r.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/")))
	if *verbose {
		fmt.Printf("running at %d\nstoring at %s\n", *port, *store)
	}

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), r))
}

func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
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

		kkey := r.Header.Get("K-API-Key")
		ksecret := r.Header.Get("K-Secret-Key")
		kpass := r.Header.Get("K-Passphrase")
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
		client := binance2.NewClient(key, secret)

		payload, err := fetchBalances(b, existing, verbose)
		if err != nil {
			response := map[string]string{"error": err.Error()}
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		// save payload
		payload.persist(path)
		if err != nil {
			response := map[string]string{"error": err.Error()}
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		go func(ctx context.Context, client *binance2.Client, path string) {
			start := time.Now().Unix()
			if kkey != "" && ksecret != "" && kpass != "" {
				// TODO: async
				// TODO: separate function
				ks := kucoin.NewApiService(
					kucoin.ApiBaseURIOption("https://api.kucoin.com"),
					kucoin.ApiKeyOption(kkey),
					kucoin.ApiSecretOption(ksecret),
					kucoin.ApiPassPhraseOption(kpass),
					kucoin.ApiKeyVersionOption(kucoin.ApiKeyVersionV2),
				)
				var klast int64 = 0
				for _, v := range existing.Kucoin {
					for _, vv := range v.Pairs {
						t := vv.LatestTrade.Time.UnixMilli()
						if t > klast {
							klast = t
						}
					}
				}
				kb, err := fetchKucoinBalance(ks, existing.Kucoin)
				if err != nil {
					fmt.Println(err)
					return
				}
				kt, err := fetchKucoinTrades(ks, klast+1, time.Now().UnixMilli(), 1, kb, verbose)
				if err != nil {
					fmt.Println(err)
					return
				}
				payload.Kucoin = kt
				payload.persist(path)
			}

			_, err := update(ctx, b, client, payload, path, verbose)
			if err != nil {
				fmt.Println(err)
				return
			}
			payload.persist(path)

			if verbose {
				fmt.Printf("%s done after %d seconds\n", r.RemoteAddr, time.Now().Unix()-start)
			}
		}(r.Context(), client, path)
		http.ServeFile(w, r, path)
	}
}

func DeleteHandler(store string, verbose bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		// no extra auth. anyone with key can delete
		path := fmt.Sprintf("%s/%s.json", store, key)

		err := os.Remove(path)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}
			fmt.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response := map[string]bool{"deleted": true}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// func fetchLocked(ctx context.Context, client binance2.Client) error {
// 	r2, err := client.NewGetSavingsFixedAndActivityPositionService().
// 		Asset("LUNA").
// 		ProjectId("CLUNA90DAYSS001").
// 		Status("HOLDING").
// 		Do(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	for _, p := range r2 {
// 		fmt.Println(p)
// 	}
// 	res, err := client.NewGetLendingPurchaseRecordService().
// 		LendingType("CUSTOMIZED_FIXED").
// 		Size(100).Current(1).
// 		StartTime(time.Now().Unix() - 2592000).
// 		EndTime(time.Now().Unix() - 3600).
// 		Do(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	for _, p := range res {
// 		fmt.Println(p.Asset, p.Amount)
// 	}
// 	// return
// 	res2, err := client.NewListSavingsFixedAndActivityProductsService().
// 		// Asset("LUNA").
// 		Type("CUSTOMIZED_FIXED").Status("ALL").
// 		Current(1).
// 		Size(100).
// 		Do(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	for _, s := range res2 {
// 		// if !strings.Contains(s.Asset, "LUNA") {
// 		// 	fmt.Println(s.ProjectId)
// 		// 	continue
// 		// }
// 		fmt.Println(s.Asset, s.ProjectId)
// 		r2, err := client.NewGetSavingsFixedAndActivityPositionService().
// 			Status("HOLDING").
// 			// Asset(s.Asset).ProjectId(s.ProjectId).
// 			// Asset("LUNA").ProjectId("Luna*30").
// 			Asset("LUNA").ProjectId("CLUNA30DAYSS001").
// 			Do(ctx)
// 		if err != nil {
// 			return err
// 		}
// 		for _, p := range r2 {
// 			fmt.Println(p)
// 		}
// 	}
// 	return nil
// }

func fetchDistributions(ctx context.Context, client *binance2.Client, symbol string, total float64, start int64, verbose bool) (int64, float64, error) {
	request := client.NewAssetDividendService().Asset(symbol).Limit(500)
	if start > 0 {
		request = request.StartTime(start + 1).EndTime(time.Now().Unix())
	}
	distributions, err := request.Do(ctx)
	if err != nil {
		if ae, ok := err.(*common.APIError); ok {
			if ae.Code == -1003 {
				// api rate limit. Wait
				// TODO: persist while waiting
				if verbose {
					fmt.Printf("[%s] Waiting for limit to refresh distributions\n", symbol)
				}
				time.Sleep(time.Minute)
				return fetchDistributions(ctx, client, symbol, total, start, verbose)
			}
		}
		err = errors.Wrap(err, fmt.Sprintf("[%s] fetching distributions", symbol))
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
	if verbose {
		fmt.Printf("[%s] %.2f distributed\n", symbol, newTotal)
	}
	if len(rows) < 1 {
		return start, newTotal, nil
	}
	return rows[0].Time, newTotal, nil
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
		return Payload{time.Now(), existing.Assets, nil}, err
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

	return Payload{time.Now(), assets, nil}, nil
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

func update(ctx context.Context, b binance.Binance, client *binance2.Client, payload Payload, path string, verbose bool) (map[string]Asset, error) {
	// persists while waiting
	pairs, err := fetchPairs()
	if err != nil {
		return nil, err
	}
	bals := payload.Assets
	var total int = 0
	// TODO: prioritize balances that have changed and are > 0
	for k, existing := range bals {
		new := existing
		// fetch trades
		// TODO: separate function
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
					// Limit:      1000,
				})
				if err != nil {
					if strings.HasPrefix(err.Error(), "-1003") {
						// api rate limit. Wait
						// persist while waiting
						go func(bals map[string]Asset, path string, total int, verbose bool) {
							// ok to ignore persist error. It will be retried
							// persist despite nothing new to update last_update
							p := Payload{time.Now(), bals, payload.Kucoin}
							err := p.persist(path)
							if err != nil {
								return
							}
							if verbose {
								fmt.Printf("%d trades saved\n", total)
							}
						}(bals, path, total, verbose)
						if verbose {
							fmt.Printf("[%s] Waiting for limit to refresh trades\n", product)
						}
						time.Sleep(time.Minute)
						// fromID not updated so it will be retried on continue
						continue
					} else {
						err = errors.Wrap(err, fmt.Sprintf("[%s] fetching trades", product))
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
		if new.Pairs == nil {
			// remove untraded
			if verbose {
				fmt.Printf("%s untraded. Removing\n", k)
			}
			delete(bals, k)
			continue
		}
		if verbose {
			fmt.Printf("[%s] fetching distributions\n", k)
		}
		// fetch distributions
		dlatest, dtotal, err := fetchDistributions(context.Background(), client, k, existing.DistributionTotal, existing.LatestDistributionTime, verbose)
		if err == nil {
			new.DistributionTotal = dtotal
			new.LatestDistributionTime = dlatest
		}
		// update asset map after fetching all trades and distributions for an asset
		bals[k] = new
	}
	if verbose {
		fmt.Printf("Fetched %d new trades since %s\n", total, payload.LastUpdate.Format("2006-01-02 3:04PM"))
	}
	return bals, err
}

func loadExisting(path string) Payload {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return Payload{time.Time{}, map[string]Asset{}, map[string]Asset{}}
	}
	var payload Payload
	json.Unmarshal(content, &payload)
	if payload.Assets == nil {
		payload.Assets = map[string]Asset{}
	}
	if payload.Kucoin == nil {
		payload.Kucoin = map[string]Asset{}
	}
	return payload
}

func fetchKucoinBalance(s *kucoin.ApiService, assets map[string]Asset) (map[string]Asset, error) {
	rsp, err := s.Accounts("", "")
	if err != nil {
		return assets, err
	}

	as := kucoin.AccountsModel{}
	if err := rsp.ReadData(&as); err != nil {
		return assets, err
	}

	new := assets
	for _, a := range as {
		bal, err := strconv.ParseFloat(a.Balance, 64)
		if err != nil {
			return new, err
		}
		if value, ok := assets[a.Currency]; ok {
			value.Balance = bal
			new[a.Currency] = value
			continue
		}
		n := Asset{}
		n.Balance = bal
		new[a.Currency] = n
	}
	return new, nil
}

func getPrice(o *kucoin.OrderModel, qty float64) (float64, error) {
	price := 0.0
	if o.Price == "0" {
		spent, err := strconv.ParseFloat(o.DealFunds, 64)
		if err != nil {
			return 0, err
		}
		price = spent / qty
		if price == 0 {
			return 0, fmt.Errorf("Unnable to get price for order %s", o.Id)
		}
		return price, nil
	}
	price, err := strconv.ParseFloat(o.Price, 64)
	if err != nil {
		return 0, err
	}
	return price, nil
}

func fetchKucoinTrades(s *kucoin.ApiService, startAt, endAt, page int64, assets map[string]Asset, verbose bool) (map[string]Asset, error) {
	if verbose {
		fmt.Printf("fetching more kucoin trades from %d page %d\n", startAt, page)
	}
	params := map[string]string{}
	if startAt > 1 {
		params["startAt"] = strconv.FormatInt(startAt, 10)
	}
	params["endAt"] = strconv.FormatInt(endAt, 10)
	params["status"] = "done"
	rsp, err := s.Orders(params, &kucoin.PaginationParam{CurrentPage: page, PageSize: 500})
	if err != nil {
		return nil, err
	}

	os := kucoin.OrdersModel{}
	pd, err := rsp.ReadPaginationData(&os)
	if err != nil {
		return nil, err
	}
	newAssets := assets
	if newAssets == nil {
		newAssets = map[string]Asset{}
	}
	earliest := endAt
	for _, o := range os {
		if o.CreatedAt < earliest {
			earliest = o.CreatedAt
		}
		filled := !o.IsActive && !o.CancelExist
		if !filled {
			continue
		}
		qty, err := strconv.ParseFloat(o.DealSize, 64)
		if err != nil {
			return nil, err
		}
		if qty <= 0 {
			continue
		}
		price, err := getPrice(o, qty)
		if err != nil {
			return nil, err
		}
		fee, err := strconv.ParseFloat(o.Fee, 64)
		if err != nil {
			return nil, err
		}
		symbols := strings.Split(o.Symbol, "-")
		asset := Asset{}
		if value, ok := assets[symbols[0]]; ok {
			asset = value
		}
		if asset.Pairs == nil {
			asset.Pairs = map[string]Pair{}
		}
		pair := Pair{}
		pair.Fees = map[string]float64{}
		pair.Fees[o.FeeCurrency] = fee
		if value, ok := asset.Pairs[symbols[1]]; ok {
			pair = value
		}
		if _, ok := pair.Fees[o.FeeCurrency]; ok {
			pair.Fees[o.FeeCurrency] += fee
		}
		if o.Side == "buy" {
			pair.BuyQty += qty
			pair.Cost += (price * qty)
		}
		if o.Side == "sell" {
			pair.SellQty += qty
			pair.Revenue += (price * qty)
		}
		if verbose {
			fmt.Printf("%s %.2f %s for %.2f at %.2f on %d\n", o.Side, qty, o.Symbol, (price * qty), price, o.CreatedAt)
		}
		t := time.UnixMilli(o.CreatedAt)
		trade := binance.Trade{}
		// trade.ID = o.Id
		trade.Time = t
		trade.IsBuyer = o.Side == "buy"
		trade.Price = price
		trade.Qty = qty
		trade.Commission = fee
		trade.CommissionAsset = o.FeeCurrency
		if pair.LatestTrade == nil || pair.LatestTrade.Time.UnixMilli() < o.CreatedAt {
			pair.LatestTrade = &trade
		}
		if pair.EarliestTrade == nil || pair.EarliestTrade.Time.UnixMilli() > o.CreatedAt {
			pair.EarliestTrade = &trade
		}

		pair.Fees[o.FeeCurrency] += fee
		asset.Pairs[symbols[1]] = pair
		newAssets[symbols[0]] = asset
	}
	if pd.TotalPage > page {
		return fetchKucoinTrades(s, startAt, endAt, page+1, newAssets, verbose)
	}
	// fetch older than earliest
	if len(os) > 0 {
		for _, a := range assets {
			for _, p := range a.Pairs {
				t := p.EarliestTrade.Time.UnixMilli()
				if t < earliest {
					earliest = t
				}
			}
		}
		return fetchKucoinTrades(s, 0, earliest-1, 1, newAssets, verbose)
	}
	return newAssets, nil
}
