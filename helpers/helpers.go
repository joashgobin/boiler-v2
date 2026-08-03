package helpers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v3/log"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/js"
)

const MySQLTimestamp = "2006-01-02 15:04:05"

func GetRandomUUID() string {
	randomUUID, err := uuid.NewRandom()
	if err != nil {
		return ""
	}
	return randomUUID.String()
}

func WasteTime(numSeconds int) {
	var duration time.Duration
	duration = time.Duration(numSeconds) * time.Second
	start := time.Now()
	for time.Since(start) < duration {
		time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
	}
}

func Background(fn func(), wg *sync.WaitGroup) {
	wg.Go(func() {
		// recover any panic
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("%v", err)
			}
		}()
		fn()
	})
	/*
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Error(fmt.Sprintf("%v", err))
				}
			}()

			fn()
		}()
	*/
}

func PrintType(v any) {
	switch v := v.(type) {
	case int:
		fmt.Printf("Value %d is of type int\n", v)
	case string:
		fmt.Printf("Value %q is of type string\n", v)
	case float64:
		fmt.Printf("Value %f is of type float64\n", v)
	default:
		fmt.Printf("Value %v is of type %T\n", v, v)
	}
}

// helper to be used in template engine to get reference to file
/*
func GetFingerprint(staticDir string) string {
	fullPath := filepath.Join(staticDir, strings.TrimPrefix(path, "/static"))
	fileInfo, err := os.Stat(fullPath)
	fp, err := GenerateFingerprint(fullPath)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	newURL := fmt.Sprintf("%s.%s%s",
		strings.TrimSuffix(path, filepath.Ext(path)),
		fp,
		filepath.Ext(path))
	log.Infof("generated fingerprint %s", newURL)
}
*/

func GenerateFingerprint(srcPath string, fileListPtr *map[string]string) (string, error) {
	err := os.MkdirAll("static/gen", 0755)
	if err != nil {
		log.Infof("failed to create directory", "static/gen")
	}

	srcContent, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}

	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/javascript", js.Minify)

	finalFolder := srcPath
	if !strings.HasPrefix(srcPath, "static/gen") {
		finalFolder = strings.Replace(srcPath, "static/", "static/gen/", -1)

	}
	// minify the file first e.g. style.min.css
	minPath := fmt.Sprintf("%s.min%s",
		strings.TrimSuffix(finalFolder, filepath.Ext(srcPath)),
		filepath.Ext(srcPath))

	mimeType := GetMimeType(srcPath)
	minifiedContent, err := m.Bytes(mimeType, srcContent)
	if err != nil {
		return "", err
	}

	hashString := FingerprintFromBuffer(minifiedContent)

	dstPath := fmt.Sprintf("%s.min.%s%s",
		strings.TrimSuffix(finalFolder, filepath.Ext(srcPath)),
		hashString,
		filepath.Ext(srcPath))

	key := strings.TrimPrefix(strings.TrimPrefix(srcPath, "static/"), "gen/")

	if FileExists(dstPath) {
		(*fileListPtr)[key] = dstPath
		return dstPath, nil
	}

	if err := os.WriteFile(dstPath, minifiedContent, 0644); err != nil {
		return "", err
	}

	log.Infof("minified file (%s) to new file: %s", minPath, dstPath)
	// map src path to dest path
	(*fileListPtr)[key] = dstPath

	return dstPath, nil
}

func GetMimeType(path string) string {
	switch {
	case strings.HasSuffix(path, ".css"):
		return "text/css"
	case strings.HasSuffix(path, ".js"):
		return "text/javascript"
	default:
		return "text/plain"
	}
}

func ReplaceSpecial(text string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]`)
	return strings.ToLower(re.ReplaceAllString(text, "-"))
}

func GetFileHash(srcPath string) string {
	fileInfo, err := os.Lstat(srcPath)
	if err != nil {
		log.Errorf("error getting file hash: %v", err)
		return ""
	}
	var uniqueString strings.Builder
	uniqueString.WriteString(srcPath)
	uniqueString.WriteString(fileInfo.ModTime().String())
	uniqueString.WriteString(strconv.FormatInt(fileInfo.Size(), 10))
	return GetXXH3(uniqueString.String())
}

func GenerateFingerprintsForFolder(folderPath string, targetFolder string, ext string, fileListPtr *map[string]string) {
	err := os.MkdirAll(targetFolder, 0755)
	if err != nil {
		log.Infof("failed to create directory %s", targetFolder)
	}
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("error reading directory (%s): %v\n", folderPath, err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ext {
			_, err = GenerateFingerprint(filepath.Join(folderPath, entry.Name()), fileListPtr)
			if err != nil {
				log.Errorf("could not generate fingerprint for file (%s): %v", entry.Name(), err)
			}
		}
	}
}

func CombineAndFingerprint(finalPath string, fileListPtr *map[string]string, files ...string) error {
	outputPath := fmt.Sprintf("%s.%d.lock", finalPath, os.Getpid())
	// Open output file for writing
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	// Process each input file
	for _, filePath := range files {
		// fmt.Println(filePath)
		inputFile, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open %s: %v", filePath, err)
		}
		defer inputFile.Close()

		// Copy file contents in chunks
		buf := make([]byte, 32*1024) // 32KB buffer
		for {
			n, err := inputFile.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("failed to read from %s: %v", filePath, err)
			}

			_, err = outputFile.Write(buf[:n])
			if err != nil {
				return fmt.Errorf("failed to write to output: %v", err)
			}
		}

		// Write separator between files
		_, err = outputFile.WriteString("\n\n")
		if err != nil {
			return fmt.Errorf("failed to write separator: %v", err)
		}
	}

	err = os.Rename(outputPath, finalPath)
	if err != nil {
		return err
	}

	// fingerprint resulting file
	_, err = GenerateFingerprint(finalPath, fileListPtr)
	if err != nil {
		return fmt.Errorf("fingerprinting error: %v", err)
	}
	return nil
}

func FileExists(filePath string) bool {
	_, err := os.Lstat(filePath)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	return false
}

func FolderExists(folderPath string) bool {
	_, err := os.Stat(folderPath)
	return err == nil
}

// helper to create a database connection pool
func OpenDB(dsn string) (*sql.DB, error) {
	// set maximum connection lifetime to prevent resource leaks
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// set connection parameters
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)

	// ping with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		// close connection before returning error
		if err := db.Close(); err != nil {
			log.Infof("failed to close database connection during error: %v", err)
		}
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func CreateDirectory(path string) error {
	if FolderExists(path) {
		return nil
	}
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return err
	}
	return nil
}

func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func CopyDir(src, dst string, skipRepeats bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Create destination directory
		destPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		if skipRepeats && FileExists(destPath) {
			// log.Infof("skipping repeat: %s", destPath)
			return nil
		}

		// Copy file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		if _, err := io.Copy(destFile, srcFile); err != nil {
			return err
		}

		return destFile.Chmod(info.Mode())
	})
}

func TouchFile(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func DeleteFile(filePath string) error {
	err := os.Remove(filePath)
	if err != nil {
		return err
	}
	return nil
}

func SaveTextToDirectory(text string, filePath string) error {
	if text == "" || filePath == "" {
		return fmt.Errorf("text content and filePath must not be empty")
	}
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	err := os.WriteFile(filePath, []byte(text), 0644)
	if err != nil {
		return fmt.Errorf("failed to write text to file: %v", err)
	}
	// log.Infof("saved text to file: %s", filePath)
	return nil
}

func RunMigration(migrationQuery string, db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := db.ExecContext(ctx, migrationQuery)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1064 {
				log.Errorf("error in migration: %v", err)
				return
			} else {
				log.Errorf("error in migration: %v", err)
				return
			}
		}
		return
	}

	/*
		if err != nil {
			log.Errorf("failed to run migration: %v", err)
		}
	*/
	_, err = result.RowsAffected()
	if err != nil {
		log.Errorf("failed to run migration: %v", err)
	}
	// log.Infof("migration executed, rows affected: %d", rowsAffected)
}

func MigrateUp(db *sql.DB, migrationQuery string, args map[string]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finalMigrationQuery := migrationQuery
	for key, value := range args {
		finalMigrationQuery = strings.ReplaceAll(finalMigrationQuery, "<"+key+">", value)
	}
	result, err := db.ExecContext(ctx, finalMigrationQuery)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1064 {
				log.Errorf("error in migration: %v", err)
				return
			} else {
				log.Errorf("error in migration: %v", err)
				return
			}
		}
		return
	}

	/*
		if err != nil {
			log.Errorf("failed to run migration: %v", err)
		}
	*/
	_, err = result.RowsAffected()
	if err != nil {
		log.Errorf("failed to run migration: %v", err)
	}
	// log.Infof("migration executed, rows affected: %d", rowsAffected)
}

func StructsToMaps(structs any) []map[string]any {
	// Convert input to slice
	rv := reflect.ValueOf(structs)
	if rv.Kind() != reflect.Slice {
		return []map[string]any{}
	}

	result := make([]map[string]any, rv.Len())

	// Process each struct in the slice
	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i)
		if elem.Kind() != reflect.Struct {
			continue
		}

		// Create map for this struct
		m := make(map[string]any)

		// Add all exported fields to map
		for j := 0; j < elem.NumField(); j++ {
			field := elem.Field(j)
			if field.CanInterface() {
				m[elem.Type().Field(j).Name] = field.Interface()
			}
		}

		result[i] = m
	}

	return result
}

func GetEnv(key string) string {
	viper.SetConfigName("config")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()

	if err != nil {
		log.Errorf("error reading config.env file: %v", err)
	}

	val := viper.GetString(strings.ToLower(key))
	// log.Infof("settings: %v", viper.AllSettings())
	// log.Infof("env %s: %s", key, val)
	if !viper.IsSet(key) {
		log.Warnf("env var not set: %s", key)
	}
	return val
}

func ShuffleSlice[T any](items *[]T) {
	// rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(*items), func(i, j int) {
		(*items)[i], (*items)[j] = (*items)[j], (*items)[i]
	})
}

func ValidateConfig(config any) error {
	// First verify it's a struct
	configType := reflect.TypeOf(config)
	if configType.Kind() != reflect.Struct {
		return errors.New("config must be a struct")
	}

	// Get the value to examine fields
	configValue := reflect.ValueOf(config)

	// Iterate through all fields
	for i := 0; i < configValue.NumField(); i++ {
		field := configValue.Field(i)
		fieldName := configType.Field(i).Name

		// Check if field is exported (starts with capital letter)
		if !field.IsValid() {
			continue
		}

		// Check if field is zero-valued
		if field.IsZero() {
			return fmt.Errorf("%s is not properly initialized", fieldName)
		}
	}

	return nil
}

func Cast[T any](value any) T {
	var def T
	newValue, ok := value.(T)
	if !ok {
		return def
	}
	return newValue
}

func To[T any](value any) T {
	var def T
	newValue, ok := value.(T)
	if !ok {
		return def
	}
	return newValue
}

func ToSlice[T any](input any) []T {
	switch v := input.(type) {
	case T:
		return []T{v}
	case []T:
		return v
	default:
		return []T{}
	}
}
