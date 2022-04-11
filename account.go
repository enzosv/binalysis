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

func SignupOrLogin(path, password string, account Account) (Account, error) {
	content, err := ioutil.ReadFile(path + "/" + account.Username)
	var existing Account
	if err != nil {
		fmt.Println(err)
		//signup
		return CreateAccount(account, password, path)
	}

	json.Unmarshal(content, &existing)
	//login
	if !checkHash(password, existing.Hash) {
		return Account{}, fmt.Errorf("Invalid password for account")
	}
	return existing, nil
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
