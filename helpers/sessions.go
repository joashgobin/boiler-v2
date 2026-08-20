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

type FlashModel struct {
	Store *session.Store
}

type FlashInterface interface {
	// Redirect redirects the user to another page with the specified message
	Redirect(c fiber.Ctx, message string, args ...any) RedirectBuilder
	// Require is a middleware that ensures that a route has certain keys in its form values
	Require(keys ...string) fiber.Handler

	// Set sets/updates value for key
	Set(c fiber.Ctx, key string, value any) error
	// SetMany sets/updates multiple values for keys associated with the current session
	SetMany(c fiber.Ctx, pairs map[string]any) error

	Prefetch(c fiber.Ctx, urls ...string)
	KeepCached(c fiber.Ctx, maxAge int)
}

var _ FlashInterface = (*FlashModel)(nil)

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

// Get returns the value for a key if exists in the current session otherwise the default value specified
func (flash *FlashModel) Get[T any](c fiber.Ctx, key string, defaultValue T) T {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		return defaultValue
	}
	value := sess.Get(key)
	if value == nil {
		return defaultValue
	}
	castedValue, ok := value.(T)
	if !ok {
		return defaultValue
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
