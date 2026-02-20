package helpers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
)

func ShowContext(c fiber.Ctx) {
	log.Infof("context: %v", c)
}
