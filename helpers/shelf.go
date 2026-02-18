package helpers

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v3/log"
)

type ShelfModelInterface interface {
	Set(key string, value string)
	Get(key string) string
	GetMany(filter string) map[string]string
	SetMany(pairs map[string]string) error
}

type ShelfModel struct {
	DB *sql.DB
}

func (s *ShelfModel) Set(key string, value string) {
	SetShelf(s.DB, key, value)
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
	log.Infof("setting in shelf: %s", key)
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
		log.Errorf("%v", sql.ErrNoRows)
		return
	}
	log.Infof("updated key-value pair: (%s, %s)", key, value)
}

func GetShelf(db *sql.DB, key string) string {
	log.Infof("retrieving from shelf: %s", key)

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
	RunMigration(strings.ReplaceAll(`
	-- Select database
USE <appName>;

-- Create table
CREATE TABLE IF NOT EXISTS shelf (
    name VARCHAR(1000) NOT NULL UNIQUE,
    value LONGTEXT NOT NULL
);
	`, "<appName>", appName), db)
}
