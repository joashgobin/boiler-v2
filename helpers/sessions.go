package helpers

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/session"
)

type FlashInterface interface {
	Push(c fiber.Ctx, message string, args ...any) error
	ClearOld(c fiber.Ctx)
	Redirect(c fiber.Ctx, route string, message string, args ...any) error
	Require(keys ...string) fiber.Handler
	RequireRedirect(redirectRoute string, keys ...string) fiber.Handler
	Get(c fiber.Ctx, key string, defaultValue ...any) any
	GetString(c fiber.Ctx, key string, defaultValue ...string) string
	GetInt(c fiber.Ctx, key string, defaultValue ...int) int
	Set(c fiber.Ctx, key string, value any) error
	SetMany(c fiber.Ctx, pairs map[string]any) error
	DeleteSession(c fiber.Ctx)
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

func (flash *FlashModel) DeleteSession(c fiber.Ctx) {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		log.Errorf("session delete error: %v", err)
	}
	if err := sess.Destroy(); err != nil {
		log.Errorf("session delete error: %v", err)
	}

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

func (flash *FlashModel) Redirect(c fiber.Ctx, route string, message string, args ...any) error {
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}
	flash.Push(c, message)
	return c.Redirect().To(route + "?show=retained")
}

func (flash *FlashModel) Push(c fiber.Ctx, message string, args ...any) error {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}

	sess.Set("flashMessage", message)

	// skip clearing flash message via locals
	sess.Set("delayFlashClear", true)

	if err := sess.Save(); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return nil
}

func (flash *FlashModel) ClearOld(c fiber.Ctx) {
	sess, err := flash.Store.Get(c)
	defer sess.Release()
	if err != nil {
		return
	}
	sess.Set("old", nil)
	if err := sess.Save(); err != nil {
		return
	}
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

		// add user to locals
		c.Locals("user", sess.Get("user"))

		csrfToken := csrf.TokenFromContext(c)
		c.Locals("csrf", csrfToken)

		// add old values to locals
		if c.Query("show") == "retained" {
			c.Locals("old", sess.Get("old"))
		} else {
			c.Locals("old", map[string]string{})
		}

		updatedSession := false

		// pass flash message to locals if indicated by Push()
		if sess.Get("delayFlashClear") != nil {
			sess.Delete("delayFlashClear")
			c.Locals("flash", sess.Get("flashMessage"))
			updatedSession = true
		} else {
			c.Locals("flash", nil)
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

func IncludeSessionOldValues(store *session.Store) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Method() != "POST" {
			return c.Next()
		}
		sess, err := store.Get(c)
		defer sess.Release()
		if err != nil {
			log.Errorf("error getting session: %v", err)
		}
		oldValues := MapFromFormBody(c, true)
		sess.Set("old", oldValues)
		if err := sess.Save(); err != nil {
			log.Errorf("error saving session: %v", err)
		}
		return c.Next()
	}
}

func (flash *FlashModel) Require(keys ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		warning, err := EnsureFiberFormFields(c, keys)
		if err != nil {
			route := c.OriginalURL()
			flash.Push(c, warning)
			return c.Redirect().To(route + "?show=retained")
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
