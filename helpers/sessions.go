package helpers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/session"
)

type RedirectBuilder struct {
	context fiber.Ctx
	message string
}

// Redirect back to the previous route
func (rb RedirectBuilder) Back() error {
	return rb.context.Redirect().WithInput().With("message", rb.message).Back(rb.context.Get("Referer"))
}

// Redirect to a particular URL
func (rb RedirectBuilder) To(route string) error {
	return rb.context.Redirect().WithInput().With("message", rb.message).To(route)
}

// Redirect to a named route
func (rb RedirectBuilder) Route(routeName string) error {
	return rb.context.Redirect().WithInput().With("message", rb.message).Route(routeName)
}

type FlashInterface interface {
	// Redirect redirects the user to another page with the specified message
	Redirect(c fiber.Ctx, message string, args ...any) RedirectBuilder

	// Require is a middleware that ensures that a route has certain keys in its form values
	Require(keys ...string) fiber.Handler

	// Get returns the value for a key if exists in the current session otherwise the default value specified
	Get(c fiber.Ctx, key string, defaultValue ...any) any
	// GetString returns the string value for a key if exists in the current session otherwise the default value specified
	GetString(c fiber.Ctx, key string, defaultValue ...string) string
	// GetInt returns the integer value for a key if exists in the current session otherwise the default value specified
	GetInt(c fiber.Ctx, key string, defaultValue ...int) int
	// Set sets/updates the value for a key associated with the current session
	Set(c fiber.Ctx, key string, value any) error
	// SetMany sets/updates multiple values for keys associated with the current session
	SetMany(c fiber.Ctx, pairs map[string]any) error

	Prefetch(c fiber.Ctx, urls ...string)
	KeepCached(c fiber.Ctx, maxAge int)
}

var _ FlashInterface = (*FlashModel)(nil)

func GetUser[T any](c fiber.Ctx, flash FlashInterface) T {
	user := flash.Get(c, "user")
	data, ok := user.(T)
	if ok {
		return data
	}
	var emptyUser T
	return emptyUser
}

func (flash *FlashModel) Prefetch(c fiber.Ctx, urls ...string) {
	var urlChunk strings.Builder
	for i := range urls {
		urlChunk.WriteString(`<link rel="prefetch" href="`)
		urlChunk.WriteString(urls[i])
		urlChunk.WriteString(`" as="document">`)
	}
	c.Locals("prefetch", urlChunk.String())
}

func (flash *FlashModel) KeepCached(c fiber.Ctx, maxAge int) {
	c.Set("Cache-Control", fmt.Sprintf("private,max-age=%d", maxAge))
}

func (flash *FlashModel) GetUser(c fiber.Ctx) any {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		return nil
	}
	value := sess.Get("user")
	return value
}

type FlashModel struct {
	Store *session.Store
}

func (flash *FlashModel) Get(c fiber.Ctx, key string, defaultValue ...any) any {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		// panic(err)
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}
	value := sess.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
	}
	return value
}

func (flash *FlashModel) GetString(c fiber.Ctx, key string, defaultValue ...string) string {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	value := sess.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	castedValue, ok := value.(string)
	if !ok {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	return castedValue
}

func (flash *FlashModel) GetInt(c fiber.Ctx, key string, defaultValue ...int) int {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	value := sess.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	castedValue, ok := value.(int)
	if !ok {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return castedValue
}

func (flash *FlashModel) Set(c fiber.Ctx, key string, value any) error {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		return err
	}
	sess.Set(key, value)
	if err := sess.Save(); err != nil {
		return err
	}
	return nil
}

func (flash *FlashModel) SetMany(c fiber.Ctx, pairs map[string]any) error {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		return err
	}
	for key, value := range pairs {
		sess.Set(key, value)
	}
	if err := sess.Save(); err != nil {
		return err
	}
	return nil
}

func IncludeSessionLocals(store *session.Store) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "private,max-age=0")

		sess, err := store.Get(c)
		defer sess.Release()
		if err != nil {
			return err
		}

		// add values to locals
		user := sess.Get("user")
		c.Locals("user", user)
		c.Locals("csrf", csrf.TokenFromContext(c))
		c.Locals("canonical", c.OriginalURL())

		c.Locals("_messages", c.Redirect().Messages())
		c.Locals("old", c.Redirect().OldInputs())
		/*
			if c.Get("X-Requested-With") != "swup" {
				c.Locals("noswup", true)
			}
		*/

		updatedSession := false

		if user == nil {
			if sess.Get("idleUpdated") == nil {
				// log.Info("updated idle timeout")
				sess.SetIdleTimeout(time.Second * 90)
				sess.Set("idleUpdated", true)
				updatedSession = true
			}
		}

		// save session once if any changes were made
		if updatedSession {
			if err := sess.Save(); err != nil {
				log.Infof("error updating session info: %v", err)
			}
		}

		return c.Next()
	}
}

func (flash *FlashModel) Require(keys ...string) fiber.Handler {
	// log.Infof("required keys: %v", keys)
	return func(c fiber.Ctx) error {
		warning, err := EnsureFiberFormFields(c, keys)
		if err != nil {
			// flash.Push(c, warning)
			return c.Redirect().WithInput().With("warning", warning).Back(c.Get("Referer"))
		}
		return c.Next()
	}
}

func (flash *FlashModel) Redirect(c fiber.Ctx, message string, args ...any) RedirectBuilder {
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}
	return RedirectBuilder{context: c, message: message}
}
