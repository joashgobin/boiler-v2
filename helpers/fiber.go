package helpers

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
)

func ShowContext(c fiber.Ctx) {
	log.Infof("context: %v", c)
}

func EnsureFiberFormFields(c fiber.Ctx, fields []string) (string, error) {
	for _, v := range fields {
		if c.FormValue(v, "") == "" || len(strings.TrimSpace(c.FormValue(v, ""))) == 0 {
			return fmt.Sprintf("Please input %s", strings.ReplaceAll(v, "-", " ")), fmt.Errorf("form: value missing: %s", v)
		}
	}
	return "", nil
}

func ParseBodyForKey(bodyData []byte, key string) map[string]string {
	body := string(bodyData)
	// Split the body into individual key-value pairs
	pairs := strings.Split(body, "&")

	// Create a map to store the key-value pairs
	data := make(map[string]string)

	// Process each pair
	for _, pair := range pairs {
		// Split each pair into key and value
		kv := strings.Split(pair, "=")

		// Skip invalid pairs
		if len(kv) != 2 {
			continue
		}

		if strings.Contains(kv[0], key) {
			// Store the key-value pair
			data[kv[0]] = kv[1]
		}
	}

	return data
}

func CompileFromBody(bodyData []byte, key string) []string {
	body := string(bodyData)
	// fmt.Printf("%v\n", body)

	pairs := strings.Split(body, "&")

	var data []string

	for _, pair := range pairs {
		kv := strings.Split(pair, "=")

		if len(kv) != 2 {
			continue
		}

		if strings.Contains(kv[0], key) {
			data = append(data, strings.ReplaceAll(kv[1], "+", " "))
		}
	}
	return data
}

func CollectFiberFormData(c fiber.Ctx, fields *[]string, multiples *[]string) string {
	var snippets string
	for _, field := range *fields {
		if slices.Contains(*multiples, field) {
			// fmt.Printf("%s\n", field)
			values := CompileFromBody(c.Body(), "options-"+ReplaceSpecial(field))
			snippets = snippets + "<p><strong>" + field + "</strong>:<ul>"
			for _, value := range values {
				snippets = snippets + "<li>" + value + "</li>"
			}
			snippets = snippets + "</ul></p>"
		} else {
			snippets = snippets + "<p><strong>" + field + "</strong>: " + c.FormValue(ReplaceSpecial(field)) + "</p>"
		}
	}
	return snippets
}

func MapFromFormBody(c fiber.Ctx, excludeEmpty bool) map[string]string {
	body := string(c.Body())

	pairs := strings.Split(body, "&")

	data := make(map[string]string, 1)

	for _, pair := range pairs {
		kv := strings.Split(pair, "=")

		if len(kv) != 2 {
			continue
		}

		if kv[0] == "csrf" {
			continue
		}

		if kv[1] == "" && excludeEmpty {
			continue
		}

		value, err := url.QueryUnescape(kv[1])
		key, err2 := url.QueryUnescape(kv[0])
		if err == nil && err2 == nil {
			data[key] = value
		}
	}
	return data
}
