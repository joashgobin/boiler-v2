package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"uuid"

	"github.com/gofiber/fiber/v3"
)

type FilesInterface interface {
	UploadImage(c fiber.Ctx, imageFormField string) (string, error)
}

type FilesModel struct {
}

var _ FilesInterface = (*FilesModel)(nil)

func (files *FilesModel) UploadImage(c fiber.Ctx, imageFormField string) (string, error) {
	file, err := c.FormFile(imageFormField)
	if err != nil {
		return "", err
	}
	filename := strings.ReplaceAll(uuid.NewV4().String(), "-", "")
	fileExt := strings.Split(file.Filename, ".")[1]
	imagePath := fmt.Sprintf("%s-%v.%s", filename, time.Now().Unix(), fileExt)

	err = c.SaveFile(file, fmt.Sprintf("./uploads/%s", imagePath))
	if err != nil {
		return "", err
	}
	return imagePath, nil
}

func SaveStructToFile[T any](item T, fileName string) error {
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("save struct to file marshall error: %v", err)
	}
	err = os.WriteFile(fileName, data, 0644)
	if err != nil {
		return fmt.Errorf("save struct to file write error: %v", err)
	}
	return nil
}

func LoadStructFromFile[T any](structExample T, fileName string) (T, error) {
	var defaultValue T
	data, err := os.ReadFile(fileName)
	if err != nil {
		return defaultValue, fmt.Errorf("load struct from file read error: %v", err)
	}
	var value T
	err = json.Unmarshal(data, &value)
	if err != nil {
		return defaultValue, fmt.Errorf("load struct from file unmarshal error: %v", err)
	}
	return value, nil
}
