package helpers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v3/log"
)

type ShelfInterface interface {
	Get(key string) string
	GetEncrypted(key string) string
	GetMany(filter string) map[string]string
	Set(key string, value string)
	SetMany(pairs map[string]string) error
	SetEncrypted(key, value string) error
}

type ShelfModel struct {
	DB *sql.DB
}

var _ ShelfInterface = (*ShelfModel)(nil)

func (s *ShelfModel) Set(key string, value string) {
	SetShelf(s.DB, key, value)
}

func Encrypt(plainText, key string) (string, error) {
	cipherText, err := encrypt(plainText, []byte(key))
	if err != nil {
		return "", fmt.Errorf("encrypt error: %v", err)
	}
	return string(cipherText), nil
}

func Decrypt(cipherText, key string) (string, error) {
	plainText, err := decrypt(cipherText, []byte(key))
	if err != nil {
		return "", fmt.Errorf("decrypt error: %v", err)
	}
	return string(plainText), nil
}

// Example usage: encrypting with AES-GCM and base64 encoding
func encrypt(plainText string, key []byte) (string, error) {
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	io.ReadFull(rand.Reader, nonce)
	// Seal concatenates the nonce and the encrypted data
	cipherText := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func decrypt(cipherTextBase64 string, key []byte) (string, error) {
	data, _ := base64.StdEncoding.DecodeString(cipherTextBase64)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonceSize := gcm.NonceSize()
	// Extract nonce and actual ciphertext
	nonce, encrypted := data[:nonceSize], data[nonceSize:]
	plainText, _ := gcm.Open(nil, nonce, encrypted, nil)
	return string(plainText), nil
}

func (s *ShelfModel) SetEncrypted(key, value string) error {
	encryptedValue, err := Encrypt(value, key)
	// log.Infof("set encrypted value for: %s with key %s", encryptedValue, key)
	if err != nil {
		return fmt.Errorf("shelf set encrypted error: %v", err)
	}
	query := `
		INSERT INTO shelf (name, value)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE
			value = VALUES(value)
		`
	result, err := s.DB.Exec(query, key, encryptedValue)
	if err != nil {
		return fmt.Errorf("shelf exec error: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("shelf affected rows error: %v", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("shelf error: no new rows set")
	}
	return nil
}

func (s *ShelfModel) GetEncrypted(key string) string {
	// log.Infof("getting decrypted value for: %s", key)
	var value string
	query := "SELECT value FROM shelf WHERE name = ?"
	err := s.DB.QueryRow(query, key).Scan(&value)
	if err != nil {
		log.Errorf("shelf encrypted value not found for key %s", key)
		return ""
	}
	decryptedValue, err := Decrypt(value, key)
	if err != nil {
		return ""
	}
	// log.Infof("got decrypted value: %s", decryptedValue)
	return decryptedValue
}

func (s *ShelfModel) Get(key string) string {
	return GetShelf(s.DB, key)
}

func (s *ShelfModel) GetMany(filter string) map[string]string {
	query := `
	SELECT name,value FROM shelf WHERE name LIKE '%` + filter + `%'
	`
	rows, err := s.DB.Query(query)
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()

	values := make(map[string]string)
	for rows.Next() {
		name := ""
		value := ""
		err = rows.Scan(&name, &value)
		if err != nil {
			log.Errorf("scan error: %v", err)
		}
		values[name] = value
	}
	return values
}

func (s *ShelfModel) SetMany(pairs map[string]string) error {
	if len(pairs) == 0 {
		return fmt.Errorf("shelf set many error: no values passed")
	}
	placeholders := make([]string, len(pairs))
	values := make([]interface{}, 0)
	count := 0
	for i, pair := range pairs {
		placeholders[count] = "(?, ?)"
		count++
		values = append(values, i, pair)
	}
	query := fmt.Sprintf(`
			INSERT INTO shelf (name, value)
			VALUES %s
			ON DUPLICATE KEY UPDATE
				value = VALUES(value)
			`, strings.Join(placeholders, ","))
	_, err := s.DB.Exec(query, values...)
	// log.Infof("inserting multiple values:\n%v", query)
	if err != nil {
		log.Errorf("multiple insert error: %v", err)
		return err
	}
	// log.Infof("updated key-value pairs: %v", pairs)
	return nil
}

func SetShelf(db *sql.DB, key string, value string) {
	// log.Infof("setting in shelf: %s", key)
	query := `
		INSERT INTO shelf (name, value)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE
			value = VALUES(value)
		`
	result, err := db.Exec(query, key, value)
	if err != nil {
		log.Errorf("failed to insert/update key (%v): %v", key, err)
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Errorf("failed to check the affected rows: %v", err)
		return
	}
	if rowsAffected == 0 {
		// log.Errorf("%v", sql.ErrNoRows)
		return
	}
	// log.Infof("updated key-value pair: (%s, %s)", key, value)
}

func GetShelf(db *sql.DB, key string) string {
	// log.Infof("retrieving from shelf: %s", key)

	var value string
	query := "SELECT value FROM shelf WHERE name = ?"
	err := db.QueryRow(query, key).Scan(&value)
	if err != nil {
		log.Errorf("shelf value not found for key %s", key)
		return ""
	}
	return value
}

func InitShelf(db *sql.DB, appName string) {
	MigrateUp(db, `
		USE <appName>;

		CREATE TABLE IF NOT EXISTS shelf (
			name VARCHAR(500) NOT NULL UNIQUE,
			value LONGTEXT NOT NULL
		);
`, map[string]string{"appName": appName})
}
