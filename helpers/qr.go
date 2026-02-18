package helpers

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	qrc "github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"
)

func GetQR(message string, jpegSavePath string) {
	// fmt.Println("generating qr for", message)
	qrc, err := qrc.New(message)
	if err != nil {
		fmt.Printf("could not generate QR Code for: %v\n", err)
		return
	}

	w, err := standard.New(jpegSavePath + ".jpeg")
	if err != nil {
		fmt.Printf("standard.New failed: %v", err)
		return
	}

	if err = qrc.Save(w); err != nil {
		fmt.Printf("could not save image: %v", err)
	}
}

func NewQR() *QR {
	CreateDirectory("./qr")
	return &QR{}
}

var _ QRInterface = (*QR)(nil)

type QR struct {
}

type QRInterface interface {
	Send(c fiber.Ctx, message string) error
}

func (qr *QR) Send(c fiber.Ctx, message string) error {
	jpegSavePath := "./qr/" + GetHash(message)
	if !FileExists(jpegSavePath + ".jpeg") {
		GetQR(message, jpegSavePath)
	}
	// c.Response().Header.Set("Cache-Control", "max-age=31536000, public")
	return c.SendFile(jpegSavePath + ".jpeg")
}
