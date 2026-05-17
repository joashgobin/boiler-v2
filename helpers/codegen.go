package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3/log"
	"github.com/pelletier/go-toml/v2"
)

func FileSubstitute(templatePath string, savePath string, values map[string]string) {
	createRecursive(filepath.Dir(savePath))
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		log.Infof("error reading template (%s): %v", templatePath, err)
		return
	}
	saveContent := string(templateContent)

	for key, value := range values {
		saveContent = strings.ReplaceAll(saveContent, fmt.Sprintf("<%s>", key), value)
	}
	err = os.WriteFile(savePath, []byte(saveContent), 0644)
	if err != nil {
		log.Infof("error saving new content to %s: %v", savePath, err)
		return
	}
}

func createRecursive(saveDir string) {
	err := os.MkdirAll(saveDir, 0755)
	if err != nil {
		log.Infof("failed to create directory (%s): %v", saveDir, err)
	}
}

func GetFieldsFromTemplateFile(templatePath string) ([]string, error) {
	re := regexp.MustCompile(`<([^>]+)>`)
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		log.Infof("error reading template (%s): %v", templatePath, err)
		return nil, err
	}
	saveContent := string(templateContent)
	var matches []string

	fields := re.FindAllStringSubmatch(saveContent, -1)
	for _, field := range fields {
		if slices.Contains(matches, field[1]) {
			continue
		}
		matches = append(matches, field[1])
	}
	return matches, nil
}

func ParseToml(filePath string) (map[string]any, error) {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %v", err)
	}
	var tomlContent map[string]any
	err = toml.Unmarshal(fileContent, &tomlContent)
	if err != nil {
		return nil, err
	}
	return tomlContent, nil
}

func ParseTomlWithFields(filePath string, fields []string) (map[string]string, error) {
	tomlContent, err := ParseToml(filePath)
	if err != nil {
		return nil, err
	}
	filteredContent := map[string]string{}
	for _, field := range fields {
		if value, exists := tomlContent[field]; exists {
			switch v := value.(type) {
			case string:
				if strings.Contains(v, "<") || strings.Contains(v, "<") {
					continue
				}
				filteredContent[field] = v
			default:
				filteredContent[field] = fmt.Sprintf("%v", v)
			}
		}
		if value, exists := tomlContent["params"].(map[string]any)[field]; exists {
			switch v := value.(type) {
			case string:
				if strings.Contains(v, "<") || strings.Contains(v, "<") {
					continue
				}
				filteredContent[field] = v
			default:
				filteredContent[field] = fmt.Sprintf("%v", v)
			}
		}
	}
	return filteredContent, nil
}
