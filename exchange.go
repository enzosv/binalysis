package main

import (
	"context"
	"strconv"
	"time"

	"github.com/Kucoin/kucoin-go-sdk"
	binance2 "github.com/adshao/go-binance/v2"
	common "github.com/adshao/go-binance/v2/common"
)

type ExchangeService interface {
	FetchBalance(ctx context.Context, assets map[string]Asset) (map[string]Asset, error)
}

type binanceService struct {
	client *binance2.Client
}

type kucoinService struct {
	client *kucoin.ApiService
}

func NewService() ExchangeService { return &binanceService{} }

func Update(ctx context.Context, dir, token string) (map[string]map[string]Asset, time.Time, error) {
	account, err := accountFromToken(dir, token)
	if err != nil {
		return nil, time.Time{}, err
	}
	exchanges := map[string]map[string]Asset{}
	var exchangeService ExchangeService
	for key, e := range account.Exchanges {
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
		}
		assets, err := exchangeService.FetchBalance(ctx, e.Assets)
		if err != nil {
			return nil, time.Time{}, err
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
