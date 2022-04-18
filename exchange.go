package main

import (
	"context"
	"strconv"
	"time"

	"github.com/Kucoin/kucoin-go-sdk"
	binance2 "github.com/adshao/go-binance/v2"
)

func Update(ctx context.Context, dir, token string) (map[string]map[string]Asset, time.Time, error) {
	account, err := accountFromToken(dir, token)
	if err != nil {
		return nil, time.Time{}, err
	}
	var binanceService *binance2.Client
	var kucoinService *kucoin.ApiService
	exchanges := map[string]map[string]Asset{}
	for key, e := range account.Exchanges {
		var assets map[string]Asset
		var err error
		// TODO: service protocol
		switch key {
		case "binance":
			binanceService = binance2.NewClient(e.APIKey, e.Secret)
			assets, err = BinanceFetchBalance(ctx, binanceService, e.Assets)
			if err != nil {
				return nil, time.Time{}, err
			}
		case "kucoin":
			kucoinService = kucoin.NewApiService(
				kucoin.ApiBaseURIOption("https://api.kucoin.com"),
				kucoin.ApiKeyOption(e.APIKey),
				kucoin.ApiSecretOption(e.Secret),
				kucoin.ApiPassPhraseOption(e.Phrase),
				kucoin.ApiKeyVersionOption(kucoin.ApiKeyVersionV2),
			)
			assets, err = KucoinFetchBalance(ctx, kucoinService, e.Assets)
			if err != nil {
				return nil, time.Time{}, err
			}
		}
		e.Assets = assets
		account.Exchanges[key] = e
		err = account.Save(dir)
		if err != nil {
			return nil, time.Time{}, err
		}
		exchanges[key] = assets
	}
	return exchanges, time.Now(), nil
}

//TODO: FetchBalance Protocol, BinanceService, KucoinService
func BinanceFetchBalance(ctx context.Context, service *binance2.Client, assets map[string]Asset) (map[string]Asset, error) {
	account, err := service.NewGetAccountService().Do(ctx)
	if err != nil {
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
		if n, ok := assets[b.Asset]; ok {
			n.Balance = free + locked
			assets[b.Asset] = n
			continue
		}
		n := Asset{}
		n.Balance = free + locked
		assets[b.Asset] = n
	}
	return assets, nil
}

func KucoinFetchBalance(ctx context.Context, service *kucoin.ApiService, assets map[string]Asset) (map[string]Asset, error) {
	rsp, err := service.Accounts("", "")
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
		if n, ok := assets[a.Currency]; ok {
			n.Balance = bal
			assets[a.Currency] = n
			continue
		}
		n := Asset{}
		n.Balance = bal
		assets[a.Currency] = n
	}
	return assets, nil
}
