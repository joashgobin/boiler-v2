package helpers

import "strings"

type SitemapInterface interface {
	Add(path string)
	Get() []string
}
type Sitemap struct {
	baseURL   string
	locations []string
}

func (s *Sitemap) Add(path string) {
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	s.locations = append(s.locations, "https://"+s.baseURL+path)
}

func (s *Sitemap) Get() []string {
	return s.locations
}

func NewSitemap(url string) *Sitemap {
	sitemap := Sitemap{baseURL: url, locations: []string{}}
	return &sitemap
}
