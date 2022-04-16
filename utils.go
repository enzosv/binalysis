package main

import (
	"crypto/sha1"
	"encoding/hex"
	"log"

	"golang.org/x/crypto/bcrypt"
)

var hmacSecret []byte = generateSecret()

func sha(text string) string {
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

func generateSecret() []byte {
	secret, err := generateRandomBytes(32)
	if err != nil {
		log.Fatal(err)
	}
	return secret
}
