package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const DIR = "."
const PASSWORD = "password"
const USERNAME = "test"

var PATH string = fmt.Sprintf("%s/%s", DIR, simpleHash(USERNAME))

func TestLifecycle(t *testing.T) {
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

	username, err := getUsernameFromToken(token)
	if !assert.Equal(t, account.Username, username, "token username mismatch") {
		return
	}
	fmt.Println("filename", USERNAME, simpleHash(USERNAME))
	// delete
	err = DeleteAccount(DIR, token)
	if err != nil {
		t.Fatalf("%v", err)
	}
}
