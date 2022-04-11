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
	Hash       string                     `json:"hash"`
	LastUpdate time.Time                  `json:"last_update"`
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
		fmt.Println(err)
		//signup
		return CreateAccount(account, password, path)
	}
	return Account{}, fmt.Errorf("account already exists with username '%s'", account.Username)
}

// TODO: update account keys, secrets

func CreateAccount(account Account, password, path string) (Account, error) {
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

func (account Account) Update(password, path string) (Account, error) {
	existing, err := Login(path, account.Username, password)
	if err != nil {
		return Account{}, err
	}
	for k, a := range account.Exchanges {
		if a.APIKey == "" || a.Secret == "" {
			// use unlink to delete
			continue
		}
		existing.Exchanges[k] = a
	}
	err = existing.Save(path)
	if err != nil {
		return Account{}, err
	}
	return existing, nil
}

func (a Account) UnlinkExchange(key, path string) {
	delete(a.Exchanges, key)
	a.Save(path)
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
