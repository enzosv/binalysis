package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

type Account struct {
	Exchanges  map[string]ExchangeAccount `json:"exchanges"`
	Username   string                     `json:"username"`
	Hash       string
	LastUpdate time.Time `json:"last_update"`
}

func hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func checkHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func Login(path, username, password string) (Account, error) {
	content, err := ioutil.ReadFile(path + "/" + username)
	if err != nil {
		return Account{}, err
	}
	var existing Account
	json.Unmarshal(content, &existing)
	if !checkHash(password, existing.Hash) {
		return Account{}, fmt.Errorf("invalid password for account")
	}
	return existing, nil
}

func Signup(path, password string, account Account) (Account, error) {
	_, err := ioutil.ReadFile(path + "/" + account.Username)
	if err != nil {
		hash, err := hash(password)
		if err != nil {
			return Account{}, err
		}
		account.Hash = hash
		err = account.Save(path)
		if err != nil {
			return Account{}, err
		}
		return account, nil
	}
	return Account{}, fmt.Errorf("account already exists with username '%s'", account.Username)
}

func (a Account) LinkExchange(e ExchangeAccount, key, path string) error {
	a.Exchanges[key] = e
	return a.Save(path)
}

func (a Account) UnlinkExchange(key, path string) error {
	delete(a.Exchanges, key)
	return a.Save(path)
}

func (a Account) Save(path string) error {
	file, err := json.Marshal(a)
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

type ExchangeAccount struct {
	APIKey string           `json:"api_key"`
	Secret string           `json:"secret"`
	Phrase string           `json:"phrase"`
	Assets map[string]Asset `json:"assets"`
}
