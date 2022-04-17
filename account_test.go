package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const DIR = "test_accounts"
const PASSWORD = "password"
const USERNAME = "test"

var PATH string = fmt.Sprintf("%s/%s", DIR, sha(USERNAME))

func TestLifecycle(t *testing.T) {
	os.Mkdir(DIR, 0760)
	if DIR != "." {
		defer os.Remove(DIR)
	}

	defer os.Remove(PATH)
	// signup
	account := Account{}
	account.Username = USERNAME
	_, err := Signup(DIR, PASSWORD, account)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// check duplicate
	_, err = Signup(DIR, PASSWORD, account)
	if err == nil {
		t.Fatalf("should not be able to create account with same username")
	}

	// login
	token, err := Login(DIR, account.Username, PASSWORD)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// test token
	username, err := getUsernameFromToken(token)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !assert.Equal(t, account.Username, username, "token username mismatch") {
		return
	}

	binance := ExchangeAccount{}
	binance.APIKey = "apikey"
	binance.Secret = "secret"

	err = LinkExchange(binance, "binance", DIR, token)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// get account
	_, _, err = GetAccountStats(DIR, token)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = UnlinkExchange("binance", DIR, token)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// delete
	err = DeleteAccount(DIR, token)
	if err != nil {
		t.Fatalf("%v", err)
	}
}
