package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Kucoin/kucoin-go-sdk"
	binance2 "github.com/adshao/go-binance/v2"
	common "github.com/adshao/go-binance/v2/common"
)

type Trade struct {
	ID              string
	Price           float64
	Qty             float64
	Commission      float64
	CommissionAsset string
	Time            time.Time
	IsBuyer         bool
}

type ExchangeService interface {
	FetchBalance(ctx context.Context, assets map[string]Asset) (map[string]Asset, error)
	FetchTrades(ctx context.Context, assets map[string]Asset, params map[string]string, verbose bool) (map[string]Asset, error)
}

type binanceService struct {
	client *binance2.Client
}

type kucoinService struct {
	client *kucoin.ApiService
}

type AssetMessage struct {
	Key    string
	Assets map[string]Asset
}

func NewService() ExchangeService { return &binanceService{} }

func Update(ctx context.Context, dir, token string) (map[string]map[string]Asset, time.Time, error) {
	account, err := accountFromToken(dir, token)
	if err != nil {
		return nil, time.Time{}, err
	}

	exchan := make(chan AssetMessage)
	defer close(exchan)
	errchan := make(chan error)
	defer close(errchan)
	for key, e := range account.Exchanges {
		go func(key string, e ExchangeAccount, ch chan AssetMessage, errch chan error) {
			var exchangeService ExchangeService
			params := map[string]string{}
			switch key {
			case "binance":
				exchangeService = &binanceService{binance2.NewClient(e.APIKey, e.Secret)}
			case "kucoin":
				exchangeService = &kucoinService{kucoin.NewApiService(
					kucoin.ApiBaseURIOption("https://api.kucoin.com"),
					kucoin.ApiKeyOption(e.APIKey),
					kucoin.ApiSecretOption(e.Secret),
					kucoin.ApiPassPhraseOption(e.Phrase),
					kucoin.ApiKeyVersionOption(kucoin.ApiKeyVersionV2),
				)}
				var klast int64 = 0
				for _, v := range e.Assets {
					for _, vv := range v.Pairs {
						t := vv.LatestTrade.Time.UnixMilli()
						if t > klast {
							klast = t
						}
					}
				}
				if klast > 0 {
					params["startAt"] = strconv.FormatInt(klast+1, 10)
				}
				params["endAt"] = strconv.FormatInt(time.Now().UnixMilli(), 10)
				params["status"] = "done"
				params["page"] = "1"
			default:
				errch <- fmt.Errorf("unhandled exchange key '%s'", key)
				return
			}
			assets, err := exchangeService.FetchBalance(ctx, e.Assets)
			if err != nil {
				errch <- err
				return
			}
			assets, err = exchangeService.FetchTrades(ctx, assets, params, false)
			if err != nil {
				errch <- err
				return
			}
			e.Assets = assets
			exchan <- AssetMessage{key, assets}
		}(key, e, exchan, errchan)
	}
	exchanges := map[string]map[string]Asset{}

	for i := 0; i < len(account.Exchanges); i++ {
		select {
		case message := <-exchan:
			exchange := account.Exchanges[message.Key]
			exchange.Assets = message.Assets
			account.Exchanges[message.Key] = exchange
			exchanges[message.Key] = message.Assets
			err := account.Save(dir)
			if err != nil {
				fmt.Println(err)
			}
		case err := <-errchan:
			fmt.Println(err)
		}
	}
	return exchanges, time.Now(), nil
}

func (service *binanceService) FetchBalance(ctx context.Context, assets map[string]Asset) (map[string]Asset, error) {
	account, err := service.client.NewGetAccountService().Do(ctx)
	if err != nil {
		if ae, ok := err.(*common.APIError); ok && ae.Code == -1003 {
			// api rate limit. Wait
			time.Sleep(time.Minute)
			return service.FetchBalance(ctx, assets)
		}
		return nil, err
	}
	for _, b := range account.Balances {
		free, err := strconv.ParseFloat(b.Free, 64)
		if err != nil {
			return nil, err
		}
		locked, err := strconv.ParseFloat(b.Locked, 64)
		if err != nil {
			return nil, err
		}
		asset := Asset{}
		if existing, ok := assets[b.Asset]; ok {
			asset = existing
		}
		asset.Balance = free + locked
		assets[b.Asset] = asset
	}
	return assets, nil
}

func (service *binanceService) FetchTrades(ctx context.Context, assets map[string]Asset, params map[string]string, verbose bool) (map[string]Asset, error) {
	return assets, nil
}

func tradeFromKucoinOrder(o *kucoin.OrderModel) (*Trade, error) {
	qty, err := strconv.ParseFloat(o.DealSize, 64)
	if err != nil {
		return nil, err
	}
	if qty <= 0 {
		return nil, nil
	}
	price, err := getPrice(o, qty)
	if err != nil {
		return nil, err
	}
	fee, err := strconv.ParseFloat(o.Fee, 64)
	if err != nil {
		return nil, err
	}
	// symbols := strings.Split(o.Symbol, "-")
	trade := Trade{}
	trade.ID = o.Id
	trade.Time = time.UnixMilli(o.CreatedAt)
	trade.IsBuyer = o.Side == "buy"
	trade.Price = price
	trade.Qty = qty
	trade.Commission = fee
	trade.CommissionAsset = o.FeeCurrency
	return &trade, nil
}

func updateAssetsWithTrade(assets map[string]Asset, trade *Trade, base, quote string) map[string]Asset {
	asset := Asset{}
	if value, ok := assets[base]; ok {
		asset = value
	}
	if asset.Pairs == nil {
		asset.Pairs = map[string]Pair{}
	}
	pair := Pair{}
	pair.Fees = map[string]float64{}
	pair.Fees[trade.CommissionAsset] = trade.Commission
	if value, ok := asset.Pairs[quote]; ok {
		pair = value
	}
	if _, ok := pair.Fees[trade.CommissionAsset]; ok {
		pair.Fees[trade.CommissionAsset] += trade.Commission
	}
	tradeValue := (trade.Price * trade.Qty)
	if trade.IsBuyer {
		pair.BuyQty += trade.Qty
		pair.Cost += tradeValue
	} else {
		pair.SellQty += trade.Qty
		pair.Revenue += tradeValue
	}
	tradeTime := trade.Time.Unix()
	if pair.LatestTrade == nil || pair.LatestTrade.Time.Unix() < tradeTime {
		pair.LatestTrade = trade
	}
	if pair.EarliestTrade == nil || pair.EarliestTrade.Time.Unix() > tradeTime {
		pair.EarliestTrade = trade
	}

	pair.Fees[trade.CommissionAsset] += trade.Commission
	asset.Pairs[quote] = pair
	assets[base] = asset
	return assets
}

func (service *kucoinService) FetchTrades(ctx context.Context, assets map[string]Asset, params map[string]string, verbose bool) (map[string]Asset, error) {
	page, err := strconv.ParseInt(params["page"], 10, 64)
	if err != nil {
		return nil, err
	}
	delete(params, "page")
	rsp, err := service.client.Orders(params, &kucoin.PaginationParam{CurrentPage: page, PageSize: 500})
	if err != nil {
		return nil, err
	}

	os := kucoin.OrdersModel{}
	pd, err := rsp.ReadPaginationData(&os)
	if err != nil {
		return nil, err
	}
	if assets == nil {
		assets = map[string]Asset{}
	}
	for _, o := range os {
		filled := !o.IsActive && !o.CancelExist
		if !filled {
			continue
		}
		trade, err := tradeFromKucoinOrder(o)
		if err != nil {
			return nil, err
		}
		if verbose {
			fmt.Printf("%s %.2f %s for %.2f at %.2f on %d\n", o.Side, trade.Qty, o.Symbol, trade.Qty*trade.Price, trade.Price, o.CreatedAt)
		}
		symbols := strings.Split(o.Symbol, "-")
		assets = updateAssetsWithTrade(assets, trade, symbols[0], symbols[1])
	}
	if pd.TotalPage > page {
		np := params
		np["page"] = strconv.FormatInt(page+1, 10)
		return service.FetchTrades(ctx, assets, np, verbose)
	}
	// TODO: Fetch older than earliest
	if len(os) > 0 {
		earliest := time.Now().UnixMilli()
		for _, a := range assets {
			for _, p := range a.Pairs {
				t := p.EarliestTrade.Time.UnixMilli()
				if t < earliest {
					earliest = t
				}
			}
		}
		np := map[string]string{}
		np["endAt"] = strconv.FormatInt(earliest-1, 10)
		np["status"] = params["status"]
		np["page"] = "1"
		return service.FetchTrades(ctx, assets, np, verbose)
	}
	return assets, nil
}

func (service *kucoinService) FetchBalance(ctx context.Context, assets map[string]Asset) (map[string]Asset, error) {
	rsp, err := service.client.Accounts("", "")
	if err != nil {
		return nil, err
	}
	as := kucoin.AccountsModel{}
	if err := rsp.ReadData(&as); err != nil {
		return nil, err
	}

	for _, a := range as {
		bal, err := strconv.ParseFloat(a.Balance, 64)
		if err != nil {
			return nil, err
		}
		asset := Asset{}
		if existing, ok := assets[a.Currency]; ok {
			asset = existing
		}
		asset.Balance = bal
		assets[a.Currency] = asset
	}
	return assets, nil
}
