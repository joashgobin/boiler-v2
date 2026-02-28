package payments

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v3/log"

	"github.com/joashgobin/boiler-v2/helpers"
)

// Environment represents a Postman environment file
type Environment struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Values               []EnvironmentVariable `json:"values"`
	PostmanVariableScope string                `json:"_postman_variable_scope"`
	PostmanExportedAt    string                `json:"_postman_exported_at"`
	PostmanExportedUsing string                `json:"_postman_exported_using"`
}

// EnvironmentVariable represents a single environment variable
type EnvironmentVariable struct {
	Key     string  `json:"key"`
	Value   string  `json:"value"`
	Type    *string `json:"type"` // Pointer to handle optional field
	Enabled bool    `json:"enabled"`
}

// Party represents debit or credit party information
type Party struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Transaction represents a single transaction
type Transaction struct {
	Amount             string    `json:"amount"`
	Currency           string    `json:"currency"`
	DisplayType        string    `json:"displayType"`
	TransactionStatus  string    `json:"transactionStatus"`
	DescriptionText    string    `json:"descriptionText"`
	ModificationDate   time.Time `json:"modificationDate"`
	TransactionRef     string    `json:"transactionReference"`
	TransactionReceipt string    `json:"transactionReceipt"`
	ExternalID         string    `json:"external_id"`
	DebitParty         []Party   `json:"debitParty"`
	CreditParty        []Party   `json:"creditParty"`
}

// Response represents the complete JSON structure
type TransactionsResponse struct {
	ExecutionID  string        `json:"executionId"`
	Transactions []Transaction `json:"TransactionList"`
}

type TransactionModel struct {
	DB *sql.DB
}

type MMGTransaction struct {
	Timestamp  time.Time
	Reference  string
	From       string
	To         string
	Amount     float64
	Currency   string
	Category   string
	Status     string
	Metadata   string
	ExternalID string
}

// Config holds application configuration
type Config struct {
	Merchant       string `json:"merchant"`
	MerchantMsisdn string `json:"merchant_msisdn"`
	SecretKey      string `json:"secret_key"`
	Amount         string `json:"amount"`
	ClientID       string `json:"client_id"`
}

// TokenParams represents token generation parameters
type TokenParams struct {
	SecretKey             string `json:"secretKey"`
	Amount                string `json:"amount"`
	MerchantID            string `json:"merchantId"`
	MerchantTransactionID string `json:"merchantTransactionId"`
	ProductDescription    string `json:"productDescription"`
	RequestInitiationTime int64  `json:"requestInitiationTime"`
	MerchantName          string `json:"merchantName"`
}

func getTransactionMeta(data []byte) (string, error) {
	// log.Infof("GET META: %v", string(data))
	r := regexp.MustCompile(`"key":\s*"(product_desc|description|descriptionText)",\s*"value":\s*"(.*?)"`)
	matches := r.FindStringSubmatch(string(data))
	if len(matches) > 1 {
		// log.Infof("found transaction meta: %s", matches[2])
		return matches[2], nil
	}
	return "", errors.New("can't find description in body")
}

func extractResourceTokenFromBody(data []byte) (string, error) {
	r := regexp.MustCompile(`"access_token":\s*"(.*?)"`)
	matches := r.FindStringSubmatch(string(data))
	if len(matches) > 0 {
		return matches[1], nil
	}
	return "", errors.New("can't find resource token in body")
}

func (m *MMGModel) LoadMMGTransactionDetails(merchantNumber int, transactionReference string, resourceToken string) {
	helpers.Background(
		func() {

			// log.Infof("LOOKUP: %s", transactionReference)

			envStr := getEnvFileString(merchantNumber)
			envMap := extractEnvMap(envStr)
			baseUrl := (*envMap)["BASE_URL_MWALLET"] + "/e-merchant-initiated-transactions/lookup?transactionId=" + transactionReference

			var urlBuilder strings.Builder
			urlBuilder.WriteString(baseUrl)
			url := urlBuilder.String()
			method := "GET"

			payload := strings.NewReader("{\"query\":\"\",\"variables\":{}}")

			client := &http.Client{}
			req, err := http.NewRequest(method, url, payload)

			if err != nil {
				log.Error(err)
				return
			}

			// send request
			req.Header.Add("x-wss-token", resourceToken)
			req.Header.Add("x-wss-mid", (*envMap)["x-wss-mid"])
			req.Header.Add("x-wss-mkey", (*envMap)["x-wss-mkey"])
			req.Header.Add("x-wss-msecret", (*envMap)["x-wss-msecret"])
			req.Header.Add("x-wss-correlationid", helpers.GetRandomUUID())
			req.Header.Add("x-api-key", (*envMap)["x-api-key"])
			req.Header.Add("Content-Type", "application/json")

			res, err := client.Do(req)
			if err != nil {
				log.Error(err)
				return
			}
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				log.Error(err)
				return
			}
			// fmt.Println(string(body))
			metadata, err := getTransactionMeta(body)
			if err != nil {
				log.Errorf("error getting transaction meta: %v", err)
			} else {
				user := ""
				query := `
					UPDATE transactions
					SET metadata = ?, user = ?
					WHERE reference = ?
					`
				result, err := m.DB.Exec(query, metadata, user, transactionReference)
				if err != nil {
					log.Errorf("failed to update transaction ref %s: %v", transactionReference, err)
					return
				}
				rowsAffected, err := result.RowsAffected()
				if err != nil {
					log.Errorf("failed to check the affected rows: %v", err)
					return
				}
				if rowsAffected == 0 {
					log.Errorf("rows affected error: %v", sql.ErrNoRows)
					return
				}
				// log.Infof("updated metadata for transaction: %s", transactionReference)
			}
		}, m.WaitGroup)
}

func (m *MMGModel) getTransactionData(data string, merchantNumber int, resourceToken string) {
	var response TransactionsResponse
	err := json.Unmarshal([]byte(data), &response)
	if err != nil {
		log.Errorf("error unmarshaling JSON: %v\n", err)
		// log.Infof("JSON: %v\n", data)
		return
	}
	// log.Infof("RESPONSE: %v", response)
	var history []MMGTransaction
	for _, transaction := range response.Transactions {
		var mmgTransaction MMGTransaction

		mmgTransaction.Amount, _ = strconv.ParseFloat(transaction.Amount, 64)
		mmgTransaction.Currency = transaction.Currency
		mmgTransaction.Status = transaction.TransactionStatus
		mmgTransaction.Category = transaction.DisplayType
		mmgTransaction.Reference = transaction.TransactionRef
		mmgTransaction.Timestamp = transaction.ModificationDate
		mmgTransaction.ExternalID = transaction.ExternalID

		for _, party := range transaction.DebitParty {
			if party.Key == "accountid" {
				mmgTransaction.From = party.Value
			}
		}

		for _, party := range transaction.CreditParty {
			if party.Key == "accountid" {
				mmgTransaction.To = party.Value
			}
		}
		history = append(history, mmgTransaction)
	}

	// only metadata is not set
	stmt, err := m.DB.Prepare(`
	INSERT INTO transactions (
		timestamp,
		reference,
		source,
		destination,
		merchant,
		amount,
		currency,
		category,
		status,
		internalid
	) VALUES (?,?,?,?,?,?,?,?,?,?)
	`)
	if err != nil {
		log.Errorf("prepare statement error: %v\n", err)
		return
	}
	defer stmt.Close()

	tx, err := m.DB.Begin()
	if err != nil {
		log.Errorf("begin transaction error: %v\n", err)
		return
	}

	for _, txn := range history {
		_, err := stmt.Exec(
			txn.Timestamp,
			txn.Reference,
			txn.From,
			txn.To,
			merchantNumber,
			txn.Amount,
			txn.Currency,
			txn.Category,
			txn.Status,
			txn.ExternalID,
		)
		if err != nil {
			tx.Rollback()
			// log.Errorf("❌ mmg transaction insert error: %v", err)
		} else {
			log.Infof("✅ inserted %s) %s: %s -> %s (%f %s)\n", txn.Reference, txn.Category, txn.From, txn.To, txn.Amount, txn.Currency)
		}
	}
	tx.Commit()
	/*
		for _, txn := range history {
			// log.Infof("QUEUE LOOKUP: %v", txn)
			m.LoadMMGTransactionDetails(merchantNumber, txn.Reference, resourceToken)
		}
	*/
}

func getEnvironmentData(merchantNumber int) (map[string]string, error) {
	data, err := os.ReadFile(getEnvFilePath(merchantNumber, "UAT"))
	if err != nil {
		fmt.Printf("error reading file: %v\n", err)
		return nil, err
	}

	// unmarshal JSON
	var env Environment
	err = json.Unmarshal(data, &env)
	if err != nil {
		fmt.Printf("error parsing JSON: %v\n", err)
		return nil, err
	}

	var pairs = make(map[string]string)

	for _, value := range env.Values {
		pairs[value.Key] = value.Value
	}
	return pairs, nil
}

func getResourceToken(db *sql.DB, merchantNumber int) string {
	return helpers.GetShelf(db, "resource-token-"+strconv.Itoa(merchantNumber))
}

func requestMMGJSON(merchantNumber int, url string, resourceToken string) (string, *http.Response) {
	// log.Infof("mmg json url: %v", url)
	// get env values
	envStr := getEnvFileString(merchantNumber)
	envMap := extractEnvMap(envStr)

	// set up http client for request
	method := "GET"
	payload := strings.NewReader("{\"query\":\"\",\"variables\":{}}")
	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		log.Error(err)
		return "", nil
	}

	// send request
	req.Header.Add("x-wss-token", resourceToken)
	req.Header.Add("x-wss-mid", (*envMap)["x-wss-mid"])
	req.Header.Add("x-wss-mkey", (*envMap)["x-wss-mkey"])
	req.Header.Add("x-wss-msecret", (*envMap)["x-wss-msecret"])
	req.Header.Add("x-wss-correlationid", helpers.GetRandomUUID())
	req.Header.Add("x-api-key", (*envMap)["x-api-key"])
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		log.Errorf("mmg json response error: %v", err)
		return "", nil
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Error("mmg json response body error: %v", err)
		return "", nil
	}
	// log.Infof("mmg json response: %v", string(body))
	return string(body), res
}

func (m *MMGModel) loadMMGTransactionHistory(merchantNumber int, blocking bool) {
	loadFunc := func() {

		// build transaction history URL from timestamps
		now := time.Now()
		toDate := now.AddDate(0, 0, 0).Format("2006-01-02T15:04:05.000Z")
		fromDate := now.AddDate(0, 0, -30).Format("2006-01-02T15:04:05.000Z")

		// get env values
		envStr := getEnvFileString(merchantNumber)
		envMap := extractEnvMap(envStr)

		baseUrl := (*envMap)["BASE_URL_MWALLET"] + "/e-merchant-initiated-transactions/txn-history"

		var urlBuilder strings.Builder
		urlBuilder.WriteString(baseUrl)
		urlBuilder.WriteString("?offset=100")
		urlBuilder.WriteString("&fromdate=" + fromDate)
		urlBuilder.WriteString("&todate=" + toDate)
		urlBuilder.WriteString("&msisdn=" + strconv.Itoa(merchantNumber))
		url := urlBuilder.String()
		// log.Infof("HISTORY URL: %s", url)

		// retrieve resource token from database
		resourceToken := getResourceToken(m.DB, merchantNumber)

		// in case resource token is empty
		if resourceToken == "" {
			log.Error("resource token returned empty")
			newToken := m.LoadNewResourceToken(merchantNumber)

			// send request
			body, _ := requestMMGJSON(merchantNumber, url, newToken)
			m.getTransactionData(body, merchantNumber, newToken)
			return
		}

		// send request
		body, res := requestMMGJSON(merchantNumber, url, resourceToken)

		// in case resource token is invalid
		if strings.Contains(string(body), "clientAuthorisationError") {
			log.Errorf("failed to use valid resource token: %v", res)

			// request new resource token
			newToken := m.LoadNewResourceToken(merchantNumber)

			// resend request
			body, _ := requestMMGJSON(merchantNumber, url, newToken)
			m.getTransactionData(body, merchantNumber, newToken)
			return
		}

		// in case of multiple user sessions
		if strings.Contains(string(body), "Multiple user session found") {
			log.Errorf("failed to use valid resource token: %v", res)

			// request new resource token
			newToken := m.LoadNewResourceToken(merchantNumber)

			// resend request
			body, _ := requestMMGJSON(merchantNumber, url, newToken)
			m.getTransactionData(body, merchantNumber, newToken)
			return
		}

		// in case authentication fails
		if strings.Contains(string(body), "Authentication failure") {
			log.Errorf("authentication failed with token: %s", resourceToken)
			newToken := m.LoadNewResourceToken(merchantNumber)

			// send request
			body, _ := requestMMGJSON(merchantNumber, url, newToken)
			m.getTransactionData(body, merchantNumber, newToken)
			return
		}

		m.getTransactionData(body, merchantNumber, resourceToken)
		// log.Infof("LOAD HISTORY TIME: %v", time.Since(now).Seconds())
	}

	if blocking {
		loadFunc()
	} else {
		helpers.Background(loadFunc, m.WaitGroup)
	}
}

func getEnvFilePath(merchantNumber int, substr string) string {
	merchantFolderPath := fmt.Sprintf("merchants/%d/", merchantNumber)
	configFiles, err := os.ReadDir(merchantFolderPath)
	if err != nil {
		log.Errorf("read merchant config error: %v", err)
		return ""
	}
	envFilePath := ""
	for _, file := range configFiles {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") && strings.Contains(file.Name(), substr) {
			// fmt.Println(file.Name())
			envFilePath = merchantFolderPath + file.Name()
			break
		}
	}

	if envFilePath == "" {
		log.Errorf("read merchant config error: could not find postman env JSON file")
		return ""
	}
	return envFilePath
}

func getEnvFileString(merchantNumber int) string {
	envStr := ""
	envFilePath := getEnvFilePath(merchantNumber, "UAT")
	envFile, err := os.ReadFile(envFilePath)
	if err != nil {
		return ""
	}
	envStr = string(envFile)
	return envStr
}

func extractEnvMap(envStr string) *map[string]string {
	mapRE := `"key":\s+"(.+)",\s+"value":\s+"(.+)",`
	matches := regexp.MustCompile(mapRE).FindAllStringSubmatch(envStr, -1)
	envMap := make(map[string]string, len(matches))
	for _, match := range matches {
		// fmt.Println(match[1], ":", match[2])
		envMap[match[1]] = match[2]
	}
	return &envMap
}

func (m *MMGModel) LoadNewResourceToken(merchantNumber int) string {
	// log.Infof("loading new resource token for %d...", merchantNumber)
	method := "POST"

	// get env values
	envStr := getEnvFileString(merchantNumber)
	envMap := extractEnvMap(envStr)

	url := (*envMap)["BASE_URL_MWALLET"] + "/e-commerce-login/mer"

	// build request payload
	var payloadBuilder strings.Builder
	payloadBuilder.WriteString("grant_type=password")

	payloadBuilder.WriteString("&api_key=" + (*envMap)["x-api-token"]) //+ helpers.Getenv("MMG_API_ALT"))
	payloadBuilder.WriteString("&username=" + strconv.Itoa(merchantNumber))
	payloadBuilder.WriteString("&password=" + (*envMap)["PASSWORD"])
	payload := strings.NewReader(payloadBuilder.String())
	// log.Infof("payload: %s", &payloadBuilder)

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		log.Errorf("load new resource token request error: %v", err)
		return ""
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	if err != nil {
		log.Errorf("load new resource token request error: %v", err)
		return ""
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Errorf("load new resource token read response error: %v", err)
		return ""
	}

	// log.Infof("response body: %v", string(body))
	token, err := extractResourceTokenFromBody(body)

	if err != nil {
		log.Error("failed to extract resource token")
		return ""
	}
	// log.Infof("new resource token: %s", token)
	helpers.SetShelf(m.DB, "resource-token-"+strconv.Itoa(merchantNumber), token)
	return token
}

func (m *MMGModel) extractMMGBalanceFromBody(body string) MMGWallet {
	// perform regex and extract merchant available balance
	pattern := regexp.MustCompile(`"(availableBalance|currentBalance)":"(\d+)"`)
	matches := pattern.FindAllStringSubmatch(body, -1)
	availableBalance := 0
	currentBalance := 0
	for _, match := range matches {
		switch match[1] {
		case "availableBalance":
			// fmt.Println("match for AB:", match)
			newAB, err := strconv.Atoi(match[2])
			if err == nil {
				availableBalance = newAB
			}
		case "currentBalance":
			// fmt.Println("match for CB:", match)
			newCB, err := strconv.Atoi(match[2])
			if err == nil {
				currentBalance = newCB
			}
		}
	}
	return MMGWallet{AvailableBalance: availableBalance, CurrentBalance: currentBalance}
}

func (m *MMGModel) GetWallet(merchantNumber int) MMGWallet {
	// get env values
	envStr := getEnvFileString(merchantNumber)
	envMap := extractEnvMap(envStr)

	baseUrl := (*envMap)["BASE_URL_MWALLET"] + "/e-merchant-initiated-transactions/balance"

	var urlBuilder strings.Builder
	urlBuilder.WriteString(baseUrl)
	merchantNumberString := strconv.Itoa(merchantNumber)
	urlBuilder.WriteString("?merchant_msisdn=" + merchantNumberString)
	url := urlBuilder.String()

	// retrieve resource token from database
	resourceToken := getResourceToken(m.DB, merchantNumber)

	// in case resource token is empty
	if resourceToken == "" {
		log.Error("resource token returned empty")
		newToken := m.LoadNewResourceToken(merchantNumber)

		// send request
		body, _ := requestMMGJSON(merchantNumber, url, newToken)
		return m.extractMMGBalanceFromBody(body)
	}

	// send request
	body, res := requestMMGJSON(merchantNumber, url, resourceToken)

	// in case resource token is invalid
	if strings.Contains(string(body), "clientAuthorisationError") {
		log.Errorf("failed to use valid resource token: %v", res)

		// request new resource token
		newToken := m.LoadNewResourceToken(merchantNumber)

		// resend request
		body, _ := requestMMGJSON(merchantNumber, url, newToken)
		return m.extractMMGBalanceFromBody(body)
	}

	// in case of multiple user sessions
	if strings.Contains(string(body), "Multiple user session found") {
		log.Errorf("failed to use valid resource token: %v", res)

		// request new resource token
		newToken := m.LoadNewResourceToken(merchantNumber)

		// resend request
		body, _ := requestMMGJSON(merchantNumber, url, newToken)
		return m.extractMMGBalanceFromBody(body)
	}

	// in case authentication fails
	if strings.Contains(string(body), "Authentication failure") {
		log.Errorf("authentication failed with token: %s", resourceToken)
		newToken := m.LoadNewResourceToken(merchantNumber)

		// send request
		body, _ := requestMMGJSON(merchantNumber, url, newToken)
		return m.extractMMGBalanceFromBody(body)
	}

	return m.extractMMGBalanceFromBody(body)
}

func loadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	config := &Config{}
	inDefaultSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if line == "[DEFAULT]" {
				inDefaultSection = true
			} else {
				inDefaultSection = false
			}
			continue
		}

		// Only process key-value pairs in DEFAULT section
		if !inDefaultSection {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "merchant":
			config.Merchant = value
		case "merchant_msisdn":
			config.MerchantMsisdn = value
		case "secret_key":
			config.SecretKey = value
		case "amount":
			config.Amount = value
		case "clientId":
			config.ClientID = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Validate required fields
	/*
		if config.Merchant == "" || config.MerchantMsisdn == "" ||
			config.SecretKey == "" || config.Amount == "" || config.ClientID == "" {
			return nil, fmt.Errorf("missing required configuration fields")
		}
	*/
	// fmt.Printf("config: %v", config)

	return config, nil
}

func loadPrivateKey(filename string) (*rsa.PrivateKey, error) {
	privateKeyData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(privateKeyData)
	if block == nil {
		return nil, fmt.Errorf("no PEM data found in key file")
	}

	// Use PKCS#8 parser
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaPriv, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA private key")
	}

	return rsaPriv, nil
}

func loadPublicKey(filename string) (*rsa.PublicKey, error) {
	publicKeyData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}

	block, _ := pem.Decode(publicKeyData)
	if block == nil {
		return nil, fmt.Errorf("no PEM data found in key file")
	}

	// Try PKIX public key format first
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// If that fails, try PKCS#1 public key format
		if parsedKey, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
			return parsedKey, nil
		}
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPub, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA public key")
	}

	return rsaPub, nil
}

func generateURL(token []byte, msisdn, clientID string) string {
	tokenStr := base64.URLEncoding.EncodeToString(token)
	link := fmt.Sprintf("https://mmgpg.mmgtest.net/mmg-pg/web/payments?token=%s&merchantId=%s&X-Client-ID=%s\n",
		tokenStr, msisdn, clientID)
	return link
}

func encrypt(data interface{}, publicKey *rsa.PublicKey) ([]byte, error) {
	jsonData, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	// log.Infof("Checkout Object:\n%s\n", jsonData)

	hash := sha256.New()
	ciphertext, err := rsa.EncryptOAEP(hash, rand.Reader, publicKey, []byte(jsonData), nil)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	return ciphertext, nil
}

func decrypt(ciphertext []byte, privateKey *rsa.PrivateKey) (map[string]interface{}, error) {
	hash := sha256.New()
	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, privateKey, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	var data map[string]interface{}
	err = json.Unmarshal(plaintext, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted data: %w", err)
	}

	// log.Infof("Decrypted response:", data)
	return data, nil
}

type MMGInterface interface {
	RegisterMerchant(merchantNumber int, merchantName string) error
	AddProduct(productCode, itemDescription string) error
	AddProducts(productMap map[string]string)
	// Checkout(userEmail string, merchantNumber int, productCode string, cost float64) string
	CheckoutOneTime(userEmail string, merchantNumber int, productDescription string, cost float64) string
	LoadHistory(merchantNumber int)
	LoadHistorySync(merchantNumber int)
	GetWallet(merchantNumber int) MMGWallet
	GetUserPurchases(userEmail string) []MMGPurchase
	GetProduct(productCode string) MMGProduct
	GetMerchant(merchantNumber int) MMGMerchant
}

type MMGModel struct {
	DB        *sql.DB
	WaitGroup *sync.WaitGroup
}

var _ MMGInterface = (*MMGModel)(nil)

type MMGMerchant struct {
	Name   string
	Number int
}

type MMGProduct struct {
	Code        string
	Description string
}

type MMGPurchase struct {
	ID          int
	User        string
	Description string
	Amount      float64
}

type MMGWallet struct {
	AvailableBalance int
	CurrentBalance   int
}

func (m *MMGModel) GetMerchant(merchantNumber int) MMGMerchant {
	query := `
	SELECT name FROM merchants
	WHERE number = ?
	`

	var merchant MMGMerchant
	merchant.Number = merchantNumber

	row := m.DB.QueryRow(query, merchantNumber)
	err := row.Scan(&merchant.Name)
	if err != nil {
		return MMGMerchant{Name: "John Doe", Number: 1234567}
	}
	return merchant
}

func (m *MMGModel) AddProducts(productMap map[string]string) {
	for productCode, productDescription := range productMap {
		m.AddProduct(productCode, productDescription)
	}
}

func (m *MMGModel) GetProduct(productCode string) MMGProduct {
	query := `
	SELECT description FROM products WHERE code = ?
	`

	var product MMGProduct
	product.Code = productCode

	row := m.DB.QueryRow(query, productCode)
	var description string
	err := row.Scan(&description)
	if err != nil {
		return MMGProduct{Code: "UNKNOWN", Description: "An Unknown Product"}
	}
	return product
}

func (m *MMGModel) GetUserPurchases(userEmail string) []MMGPurchase {
	var purchases []MMGPurchase

	query := `
	SELECT t.internalid, t.amount, p.user, p.description
	FROM transactions t
	JOIN purchases p
	ON p.id = t.internalid
	WHERE p.user = ?
	`
	rows, err := m.DB.Query(query, userEmail)
	if err != nil {
		log.Errorf("mmg user purchases query error: %v", err)
		return purchases
	}
	defer rows.Close()

	for rows.Next() {
		var purchase MMGPurchase
		err := rows.Scan(
			&purchase.ID,
			&purchase.Amount,
			&purchase.User,
			&purchase.Description,
		)
		if err != nil {
			log.Errorf("mmg user purchases scan error: %v", err)
		}
		purchases = append(purchases, purchase)
	}
	if err := rows.Err(); err != nil {
		log.Errorf("mmg user purchases rows error: %v", err)
		return purchases
	}
	return purchases
}

func (m *MMGModel) LoadHistory(merchantNumber int) {
	m.loadMMGTransactionHistory(merchantNumber, false)
}

func (m *MMGModel) LoadHistorySync(merchantNumber int) {
	m.loadMMGTransactionHistory(merchantNumber, true)
}

func (m *MMGModel) AddProduct(productCode, itemDescription string) error {

	query := `
	INSERT INTO products (code,description)
	VALUES (?,?)
	`

	result, err := m.DB.Exec(query, productCode, itemDescription)
	if err != nil {
		return fmt.Errorf("add product exec error: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("add product rows error: %v", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("add product error: product already added")
	}

	return nil
}

func (m *MMGModel) RegisterMerchant(merchantNumber int, merchantName string) error {

	query := `
	INSERT INTO merchants (name,number)
	VALUES (?,?)
	`

	result, err := m.DB.Exec(query, merchantName, merchantNumber)
	if err != nil {
		return fmt.Errorf("register merchant exec error: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("register merchant rows error: %v", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("register merchant error: merchant already registered")
	}

	return nil
}

/*
func (m *MMGModel) Checkout(userEmail string, merchantNumber int, productCode string, cost float64) string {
	_, url := initiateCheckout(userEmail, merchantNumber, m.GetMerchant(merchantNumber).Name, productCode, cost)
	return url
}
*/

func (m *MMGModel) CheckoutOneTime(userEmail string, merchantNumber int, productDescription string, cost float64) string {
	url := m.initiateCheckout(userEmail, merchantNumber, m.GetMerchant(merchantNumber).Name, productDescription, cost)
	return url
}

func (m *MMGModel) initiateCheckout(userEmail string, merchantNumber int, merchantName, productDescription string, cost float64) string {
	config, err := loadConfig(fmt.Sprintf("merchants/%d/setup.cfg", merchantNumber))
	if err != nil {
		log.Fatal(err)
	}

	publicKey, err := loadPublicKey(fmt.Sprintf("merchants/%s/keys/%s.public.pem", config.MerchantMsisdn, config.MerchantMsisdn))
	if err != nil {
		log.Fatal(err)
	}

	timestamp := time.Now().Unix()
	internalTransactionID := fmt.Sprint(timestamp)

	query := `
	INSERT INTO purchases (id,user,description)
	VALUES (?,?,?)
	`

	result, err := m.DB.Exec(query, timestamp, userEmail, productDescription)
	if err != nil {
		log.Errorf("mmg purchase query exec error: %v", err)
		return "/"
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Errorf("mmg purchase query rows error: %v", err)
		return "/"
	}
	if rowsAffected == 0 {
		log.Errorf("mmg purchase rows error: no purchases inserted")
		return "/"
	}

	tokenParams := TokenParams{
		SecretKey:             config.SecretKey,
		Amount:                strconv.FormatFloat(cost, 'g', -1, 64),
		MerchantID:            config.MerchantMsisdn,
		MerchantTransactionID: internalTransactionID,
		ProductDescription:    productDescription,
		RequestInitiationTime: timestamp,
		MerchantName:          merchantName,
	}

	token, err := encrypt(tokenParams, publicKey)
	if err != nil {
		log.Fatal(err)
		return "/"
	}

	return generateURL(token, config.MerchantMsisdn, config.ClientID)
}

func NewMMG(db *sql.DB, wg *sync.WaitGroup, appName string) *MMGModel {

	// create database
	helpers.RunMigration(strings.ReplaceAll(`
USE <appName>;

CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER 	NOT NULL PRIMARY KEY AUTO_INCREMENT,
    timestamp 	DATETIME NOT NULL,
	reference 	VARCHAR(20) NOT NULL UNIQUE,
	source      VARCHAR(20) NOT NULL,
	destination VARCHAR(20) NOT NULL,
	merchant	INTEGER,
	amount		DECIMAL(10,2) NOT NULL,
	currency 	VARCHAR(5) NOT NULL,
	category  	VARCHAR(30) NOT NULL,
	status    	VARCHAR(20) NOT NULL,
    metadata 	VARCHAR(100),
    user 		VARCHAR(100),
    expiration_date 	DATETIME,
	internalid 	VARCHAR(40)
);

CREATE TABLE IF NOT EXISTS purchases (
	id INTEGER UNIQUE,
	user VARCHAR(100),
	description LONGTEXT
);

CREATE TABLE IF NOT EXISTS merchants (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(100) NOT NULL,
	number INTEGER NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS products (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(100) NOT NULL UNIQUE,
	description VARCHAR(300) NOT NULL
);

	`, "<appName>", appName), db)

	return &MMGModel{DB: db, WaitGroup: wg}
}
