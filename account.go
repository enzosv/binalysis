package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

type Account struct {
	Exchanges  map[string]ExchangeAccount `json:"exchanges"`
	Username   string                     `json:"username"`
	Hash       string                     `json:"hash"`
	LastUpdate time.Time                  `json:"last_update"`
}

type ExchangeAccount struct {
	APIKey string           `json:"api_key"`
	Secret string           `json:"secret"`
	Phrase string           `json:"phrase"`
	Assets map[string]Asset `json:"assets"`
}

type AccountError struct {
	HTTPCode int
	Message  string
}

func (e *AccountError) Error() string {
	return fmt.Sprintf("%d:%s", e.HTTPCode, e.Message)
}

func simpleHash(text string) string {
	h := sha1.New()
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

func hash(text string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(text), 14)
	return string(bytes), err
}

func checkHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func Login(path, username, password string) (string, error) {
	content, err := ioutil.ReadFile(path + "/" + simpleHash(username))
	if err != nil {
		// also consider err.(*os.PathError)
		if errors.Is(err, os.ErrNotExist) {
			return "", &AccountError{http.StatusNotFound, fmt.Sprintf("account with username '%s' does not exist", username)}
		}
		return "", err
	}
	var existing Account
	json.Unmarshal(content, &existing)
	if !checkHash(password, existing.Hash) {
		return "", &AccountError{http.StatusUnauthorized, fmt.Sprintf("invalid password for account '%s'", username)}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": existing.Username,
	})
	return token.SignedString(hmacSecret)
}

func getUsernameFromToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return hmacSecret, nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	if username, ok := claims["username"]; ok {
		return fmt.Sprintf("%v", username), nil
	}
	return "", fmt.Errorf("invalid token")
}

func Signup(path, password string, account Account) (Account, error) {
	if strings.Contains(account.Username, "/") {
		return Account{}, &AccountError{http.StatusBadRequest, "username must not contain '/'"}
	}
	if _, err := os.Stat(path + "/" + simpleHash(account.Username)); !errors.Is(err, os.ErrNotExist) {
		return Account{}, &AccountError{http.StatusBadRequest, fmt.Sprintf("account already exists with username '%s'", account.Username)}
	}
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

func (a Account) LinkExchange(e ExchangeAccount, key, path string) error {
	a.Exchanges[key] = e
	return a.Save(path)
}

func (a Account) UnlinkExchange(key, path string) error {
	delete(a.Exchanges, key)
	return a.Save(path)
}

// consider a nosql database
func (a Account) Save(path string) error {
	file, err := json.Marshal(a)
	if err != nil {
		err = errors.Wrap(err, "encoding")
		return err
	}
	// TODO: encrypt
	err = ioutil.WriteFile(path+"/"+simpleHash(a.Username), file, 0644)
	if err != nil {
		err = errors.Wrap(err, "persisting")
		return err
	}
	return nil
}
