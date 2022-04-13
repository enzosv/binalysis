package main

import (
	"context"
	"strconv"

	"github.com/Kucoin/kucoin-go-sdk"
	binance2 "github.com/adshao/go-binance/v2"
)

//TODO: FetchBalance Protocol, BinanceService, KucoinService
func (e ExchangeAccount) BinanceFetchBalance(ctx context.Context, service *binance2.Client) (map[string]Asset, error) {
	account, err := service.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, err
	}
	assets := map[string]Asset{}
	for _, b := range account.Balances {
		free, err := strconv.ParseFloat(b.Free, 64)
		if err != nil {
			return nil, err
		}
		locked, err := strconv.ParseFloat(b.Locked, 64)
		if err != nil {
			return nil, err
		}
		n := Asset{}
		n.Balance = free + locked
		assets[b.Asset] = n
	}
	return assets, nil
}

func (e ExchangeAccount) KucoinFetchBalance(ctx context.Context, service *kucoin.ApiService) (map[string]Asset, error) {
	rsp, err := service.Accounts("", "")
	if err != nil {
		return nil, err
	}
	assets := map[string]Asset{}
	as := kucoin.AccountsModel{}
	if err := rsp.ReadData(&as); err != nil {
		return nil, err
	}

	for _, a := range as {
		bal, err := strconv.ParseFloat(a.Balance, 64)
		if err != nil {
			return nil, err
		}
		n := Asset{}
		n.Balance = bal
		assets[a.Currency] = n
	}
	return assets, nil
}
