package helpers

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3/log"
	brrr "github.com/molecule-man/go-brrr"
)

var compressSet = map[string]bool{
	".css":         true,
	".js":          true,
	".mjs":         true,
	".svg":         true,
	".ttf":         true,
	".txt":         true,
	".xml":         true,
	".webmanifest": true,
}

func GenerateBrotli(dirPath string, exceptions ...string) error {
	// log.Printf("compressing files in %s...", dirPath)

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || strings.HasSuffix(path, ".br") || strings.HasSuffix(path, ".gz") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !compressSet[ext] {
			return nil
		}

		destPath := path + ".br"

		for _, exception := range exceptions {
			// if path contains exception
			if strings.Contains(path, exception) {
				return nil
			}
		}

		if FileExists(destPath) {
			// log.Infof("skipping: %s", info.Name())
			return nil
		}

		log.Infof("compressing: %s", info.Name())

		srcData, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var compressed bytes.Buffer
		writer, err := brrr.NewWriter(&compressed, 11)
		if err != nil {
			return err
		}

		if _, err := io.Copy(writer, bytes.NewReader(srcData)); err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}

		err = os.WriteFile(destPath, compressed.Bytes(), 0644)
		if err != nil {
			return err
		}

		return nil
	})
}
