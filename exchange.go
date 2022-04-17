package main

import (
	"context"
	"strconv"

	"github.com/Kucoin/kucoin-go-sdk"
	binance2 "github.com/adshao/go-binance/v2"
)

//TODO: FetchBalance Protocol, BinanceService, KucoinService

func FetchBalances(ctx context.Context, dir, token string) (map[string]map[string]Asset, error) {
	account, err := accountFromToken(dir, token)
	if err != nil {
		return nil, err
	}
	response := map[string]map[string]Asset{}
	for key, e := range account.Exchanges {
		var assets map[string]Asset
		var err error
		switch key {
		case "binance":
			service := binance2.NewClient(e.APIKey, e.Secret)
			assets, err = BinanceFetchBalance(ctx, service, e.Assets)
			if err != nil {
				return nil, err
			}
		case "kucoin":
			service := kucoin.NewApiService(
				kucoin.ApiBaseURIOption("https://api.kucoin.com"),
				kucoin.ApiKeyOption(e.APIKey),
				kucoin.ApiSecretOption(e.Secret),
				kucoin.ApiPassPhraseOption(e.Phrase),
				kucoin.ApiKeyVersionOption(kucoin.ApiKeyVersionV2),
			)
			assets, err = KucoinFetchBalance(ctx, service, e.Assets)
			if err != nil {
				return nil, err
			}
		}
		e.Assets = assets
		account.Exchanges[key] = e
		err = account.Save(dir)
		if err != nil {
			return nil, err
		}
		response[key] = assets
	}
	return response, nil
}

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
