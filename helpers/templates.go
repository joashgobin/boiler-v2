package helpers

import (
	"github.com/gofiber/fiber/v3"
)

func HTMLMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		return c.Next()
	}
}
