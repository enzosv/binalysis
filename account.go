package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/pkg/errors"
)

type Account struct {
	Exchanges  map[string]ExchangeAccount `json:"exchanges"`
	Username   string                     `json:"-"`
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
	return fmt.Sprintf("%d: %s", e.HTTPCode, e.Message)
}

func (a Account) path(store string) string {
	return fmt.Sprintf("%s/%s", store, sha(a.Username))
}

func (a Account) token() (string, error) {
	// https: //openid.net/specs/openid-connect-core-1_0.html#IDToken
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": a.Username,
		"exp": time.Now().Add(time.Hour * time.Duration(1)).Unix(),
		"iat": time.Now().Unix(),
		"iss": "binalysis.enzosv.xyz",
		"aud": "binalysis.enzosv.xyz",
	})
	return token.SignedString(hmacSecret)
}

// consider a nosql database
func (a Account) Save(dir string) error {
	a.LastUpdate = time.Now()
	file, err := json.Marshal(a)
	if err != nil {
		err = errors.Wrap(err, "encoding")
		return err
	}
	// TODO: encrypt
	err = ioutil.WriteFile(a.path(dir), file, 0644)
	if err != nil {
		err = errors.Wrap(err, "persisting")
		return err
	}
	return nil
}

func accountFromToken(dir, token string) (Account, error) {
	username, err := getUsernameFromToken(token)
	if err != nil {
		return Account{}, nil
	}
	return loadAccount(dir, username)
}

func loadAccount(dir, username string) (Account, error) {
	content, err := ioutil.ReadFile(dir + "/" + sha(username))
	if err != nil {
		// also consider err.(*os.PathError)
		if errors.Is(err, os.ErrNotExist) {
			return Account{}, &AccountError{http.StatusNotFound, fmt.Sprintf("account with username '%s' does not exist", username)}
		}
		return Account{}, err
	}
	var existing Account
	err = json.Unmarshal(content, &existing)
	if err != nil {
		return Account{}, &AccountError{http.StatusInternalServerError, err.Error()}
	}
	existing.Username = username
	return existing, nil
}

func Login(dir, username, password string) (string, error) {
	existing, err := loadAccount(dir, username)
	if err != nil {
		return "", err
	}
	if !checkHash(password, existing.Hash) {
		return "", &AccountError{http.StatusUnauthorized, fmt.Sprintf("invalid password for account '%s'", username)}
	}
	existing.Username = username
	return existing.token()
}

func DeleteAccount(dir, token string) error {
	username, err := getUsernameFromToken(token)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/%s", dir, sha(username))
	return os.Remove(path)
}

func GetAccountStats(dir, token string) (map[string]map[string]Asset, time.Time, error) {
	account, err := accountFromToken(dir, token)
	if err != nil {
		return nil, time.Time{}, err
	}
	assets := map[string]map[string]Asset{}
	for key, exchange := range account.Exchanges {
		assets[key] = exchange.Assets
	}
	return assets, account.LastUpdate, nil
}

func getUsernameFromToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
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
	if username, ok := claims["sub"]; ok {
		return fmt.Sprintf("%v", username), nil
	}
	return "", fmt.Errorf("invalid token")
}

func Signup(dir, password string, account Account) (string, error) {
	_, err := loadAccount(dir, account.Username)
	if err == nil {
		return "", &AccountError{http.StatusBadRequest, fmt.Sprintf("account already exists with username '%s'", account.Username)}
	}
	hash, err := hash(password)
	if err != nil {
		return "", err
	}
	account.Hash = hash
	err = account.Save(dir)
	if err != nil {
		return "", err
	}
	return account.token()
}

func LinkExchange(e ExchangeAccount, key, dir, token string) error {
	a, err := accountFromToken(dir, token)
	if err != nil {
		return err
	}
	if a.Exchanges == nil {
		a.Exchanges = map[string]ExchangeAccount{}
	}
	a.Exchanges[key] = e
	return a.Save(dir)
}

func UnlinkExchange(key, dir, token string) error {
	a, err := accountFromToken(dir, token)
	if err != nil {
		return err
	}
	delete(a.Exchanges, key)
	return a.Save(dir)
}
