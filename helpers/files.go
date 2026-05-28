package helpers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
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
	filename := strings.Replace(uuid.New().String(), "-", "", -1)
	fileExt := strings.Split(file.Filename, ".")[1]
	image := fmt.Sprintf("%s-%v.%s", filename, time.Now().Unix(), fileExt)

	err = c.SaveFile(file, fmt.Sprintf("./uploads/%s", image))
	if err != nil {
		return "", err
	}
	return image, nil
}
