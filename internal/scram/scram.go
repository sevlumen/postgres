package scram

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const mechanism = "SCRAM-SHA-256"

// Client implements the SCRAM-SHA-256 exchange used by PostgreSQL.
type Client struct {
	password                string
	nonce                   string
	clientFirstBare         string
	serverFirst             string
	expectedServerSignature []byte
}

func New(username, password string) (*Client, error) {
	random := make([]byte, 18)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("scram: generate nonce: %w", err)
	}
	return NewWithNonce(username, password, base64.RawStdEncoding.EncodeToString(random))
}

func NewWithNonce(username, password, nonce string) (*Client, error) {
	if nonce == "" || strings.ContainsAny(nonce, ",\x00") {
		return nil, errors.New("scram: invalid nonce")
	}
	username = strings.ReplaceAll(strings.ReplaceAll(username, "=", "=3D"), ",", "=2C")
	bare := "n=" + username + ",r=" + nonce
	return &Client{password: password, nonce: nonce, clientFirstBare: bare}, nil
}

func (c *Client) Mechanism() string    { return mechanism }
func (c *Client) FirstMessage() string { return "n,," + c.clientFirstBare }

func (c *Client) Continue(serverFirst string) (string, error) {
	attributes, err := parseAttributes(serverFirst)
	if err != nil {
		return "", err
	}
	serverNonce := attributes["r"]
	if !strings.HasPrefix(serverNonce, c.nonce) || len(serverNonce) <= len(c.nonce) {
		return "", errors.New("scram: server nonce does not extend client nonce")
	}
	salt, err := base64.StdEncoding.DecodeString(attributes["s"])
	if err != nil {
		return "", fmt.Errorf("scram: decode salt: %w", err)
	}
	iterations, err := strconv.Atoi(attributes["i"])
	if err != nil || iterations < 4096 || iterations > 1_000_000 {
		return "", errors.New("scram: invalid iteration count")
	}

	clientFinalWithoutProof := "c=biws,r=" + serverNonce
	authMessage := c.clientFirstBare + "," + serverFirst + "," + clientFinalWithoutProof
	saltedPassword := pbkdf2SHA256([]byte(c.password), salt, iterations, sha256.Size)
	clientKey := hmacSHA256(saltedPassword, []byte("Client Key"))
	storedKey := sha256.Sum256(clientKey)
	clientSignature := hmacSHA256(storedKey[:], []byte(authMessage))
	proof := make([]byte, len(clientKey))
	for i := range clientKey {
		proof[i] = clientKey[i] ^ clientSignature[i]
	}
	serverKey := hmacSHA256(saltedPassword, []byte("Server Key"))
	c.expectedServerSignature = hmacSHA256(serverKey, []byte(authMessage))
	c.serverFirst = serverFirst
	return clientFinalWithoutProof + ",p=" + base64.StdEncoding.EncodeToString(proof), nil
}

func (c *Client) Final(serverFinal string) error {
	attributes, err := parseAttributes(serverFinal)
	if err != nil {
		return err
	}
	if serverError := attributes["e"]; serverError != "" {
		return fmt.Errorf("scram: server rejected authentication: %s", serverError)
	}
	signature, err := base64.StdEncoding.DecodeString(attributes["v"])
	if err != nil {
		return fmt.Errorf("scram: decode server signature: %w", err)
	}
	if len(c.expectedServerSignature) == 0 || subtle.ConstantTimeCompare(signature, c.expectedServerSignature) != 1 {
		return errors.New("scram: server signature mismatch")
	}
	return nil
}

func parseAttributes(message string) (map[string]string, error) {
	result := make(map[string]string)
	for _, item := range strings.Split(message, ",") {
		if len(item) < 3 || item[1] != '=' {
			return nil, errors.New("scram: malformed attribute")
		}
		key := item[:1]
		if _, exists := result[key]; exists {
			return nil, errors.New("scram: duplicate attribute")
		}
		result[key] = item[2:]
	}
	return result, nil
}

func hmacSHA256(key, value []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(value)
	return h.Sum(nil)
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLength int) []byte {
	hashLength := sha256.Size
	blocks := (keyLength + hashLength - 1) / hashLength
	result := make([]byte, 0, blocks*hashLength)
	for block := 1; block <= blocks; block++ {
		input := make([]byte, len(salt)+4)
		copy(input, salt)
		input[len(salt)] = byte(block >> 24)
		input[len(salt)+1] = byte(block >> 16)
		input[len(salt)+2] = byte(block >> 8)
		input[len(salt)+3] = byte(block)
		u := hmacSHA256(password, input)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			u = hmacSHA256(password, u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		result = append(result, t...)
	}
	return result[:keyLength]
}
