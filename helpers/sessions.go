package helpers

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/session"
)

type FlashInterface interface {
	// Push adds a message with optional arguments to the set of flash messages
	Push(c fiber.Ctx, message string, args ...any) error

	Require(keys ...string) fiber.Handler
	RequireRedirect(redirectRoute string, keys ...string) fiber.Handler

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
	c.Locals("prefetch", urls)
}

func (flash *FlashModel) KeepCached(c fiber.Ctx, maxAge int) {
	c.Set("Cache-Control", fmt.Sprintf("private,max-age=%d", maxAge))
}

func (flash *FlashModel) GetUser(c fiber.Ctx) interface{} {
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

func (flash *FlashModel) Push(c fiber.Ctx, message string, args ...any) error {
	/*
		sess, err := flash.Store.Get(c)
		defer sess.Release()
		if err != nil {
			return err
		}
	*/
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}

	c.Locals("flash", message)

	/*
		sess.Set("flashMessage", message)

		// skip clearing flash message via locals
		sess.Set("delayFlashClear", true)

		if err := sess.Save(); err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
	*/
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

		// add session to locals
		c.Locals("session", sess)

		// add values to locals
		c.Locals("user", sess.Get("user"))
		c.Locals("csrf", csrf.TokenFromContext(c))
		c.Locals("_messages", c.Redirect().Messages())
		// log.Infof("messages: %v", c.Redirect().Messages())
		c.Locals("old", c.Redirect().OldInputs())

		updatedSession := false

		if sess.Get("user") == nil {
			sess.SetIdleTimeout(time.Minute * 2)
			updatedSession = true
		}

		// pass flash message to locals if indicated by Push()
		/*
			if sess.Get("delayFlashClear") != nil {
				sess.Delete("delayFlashClear")
				c.Locals("flash", sess.Get("flashMessage"))
				updatedSession = true
			} else {
				c.Locals("flash", nil)
			}
		*/

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
	return func(c fiber.Ctx) error {
		warning, err := EnsureFiberFormFields(c, keys)
		if err != nil {
			// flash.Push(c, warning)
			return c.Redirect().WithInput().With("warning", warning).Back()
		}
		return c.Next()
	}
}

func (flash *FlashModel) RequireRedirect(redirectRoute string, keys ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		warning, err := EnsureFiberFormFields(c, keys)
		if err != nil {
			flash.Push(c, warning)
			return c.Redirect().To(redirectRoute + "?show=retained")
		}
		return c.Next()
	}
}
