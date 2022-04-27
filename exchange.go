package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Kucoin/kucoin-go-sdk"
	binance2 "github.com/adshao/go-binance/v2"
	common "github.com/adshao/go-binance/v2/common"
)

type ExchangeService interface {
	FetchBalance(ctx context.Context, assets map[string]Asset) (map[string]Asset, error)
	FetchTrades(ctx context.Context, assets map[string]Asset) (map[string]Asset, error)
}

type binanceService struct {
	client *binance2.Client
}

type kucoinService struct {
	client *kucoin.ApiService
}

type ExchangeMessage struct {
	Key      string
	Exchange ExchangeAccount
}

func NewService() ExchangeService { return &binanceService{} }

func Update(ctx context.Context, dir, token string) (map[string]map[string]Asset, time.Time, error) {
	account, err := accountFromToken(dir, token)
	if err != nil {
		return nil, time.Time{}, err
	}
	var exchangeService ExchangeService
	exchan := make(chan ExchangeMessage)
	errchan := make(chan error)
	for key, e := range account.Exchanges {
		go func(key string, e ExchangeAccount, ch chan ExchangeMessage, errch chan error) {
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
				errch <- err
				return
			}
			e.Assets = assets
			exchan <- ExchangeMessage{key, e}
		}(key, e, exchan, errchan)
	}
	exchanges := map[string]map[string]Asset{}

	for i := 0; i < len(account.Exchanges); i++ {
		select {
		case message := <-exchan:
			account.Exchanges[message.Key] = message.Exchange
			exchanges[message.Key] = message.Exchange.Assets
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

func (service *binanceService) FetchTrades(ctx context.Context, assets map[string]Asset) (map[string]Asset, error) {
	return assets, nil
}

func (service *kucoinService) FetchTrades(ctx context.Context, assets map[string]Asset) (map[string]Asset, error) {
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
