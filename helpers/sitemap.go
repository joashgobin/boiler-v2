package helpers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pahanini/go-sitemap-generator"
)

type SitemapInterface interface {
	Add(path string)
	Get(c fiber.Ctx) error
}
type Sitemap struct {
	baseURL   string
	Generator *sitemap.Generator
	locations []string
}

func (s *Sitemap) Add(path string) {
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	s.locations = append(s.locations, path)
}

func (s *Sitemap) Get(c fiber.Ctx) error {
	s.Generator.Open()
	for _, location := range s.locations {
		url := s.baseURL + location
		if !strings.HasSuffix(location, "http://") {
			url = "https://" + s.baseURL + location
		}
		s.Generator.Add(sitemap.URL{Loc: url, Priority: "0.5", LastMod: time.Now().Format(time.RFC3339)})
	}
	s.Generator.Close()
	return c.SendFile("./sitemap/sitemap.xml")
}

func NewSitemap(url string) *Sitemap {
	sm := sitemap.New(sitemap.Options{
		Dir:     "sitemap",
		BaseURL: url,
	})
	return &Sitemap{Generator: sm, baseURL: url}
}
