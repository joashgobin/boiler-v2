package core

import (
	"bytes"
	"database/sql"
	"embed"
	"encoding/gob"
	"fmt"
	"html/template"
	ht "html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3/extractors"

	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/template/html/v3"
	"github.com/joashgobin/boiler-v2/core/models"
	"github.com/joashgobin/boiler-v2/email"
	"github.com/joashgobin/boiler-v2/helpers"
	"github.com/joashgobin/boiler-v2/payments"

	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/etag"
	"github.com/gofiber/fiber/v3/middleware/idempotency"
	"github.com/gofiber/fiber/v3/middleware/logger"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3/middleware/pprof"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/gofiber/storage/valkey"
)

type Base struct {
	// public variables
	DB           *sql.DB
	Store        *session.Store
	Engine       *html.Engine
	WaitGroup    *sync.WaitGroup
	ImageChannel *chan *helpers.SafeImage
	Anchor       string

	// public interfaces
	Users   models.UserModelInterface
	Shelf   helpers.ShelfInterface
	Flash   helpers.FlashInterface
	Files   helpers.FilesInterface
	Bank    helpers.BankInterface
	MMG     payments.MMGInterface
	Mail    email.MailInterface
	QR      helpers.QRInterface
	SiteMap helpers.SitemapInterface

	// private variables
	isProd bool
	domain string
	port   string
}

type AppConfig struct {
	User                   string
	IP                     string
	Port                   string
	AppName                string
	FuncMap                map[string]interface{}
	IsProduction           bool
	ReduceMemoryUsage      bool
	SessionIdleTimeout     time.Duration
	SessionAbsoluteTimeout time.Duration
	Templates              *embed.FS
	SiteInfo               *map[string]string
}

func (base *Base) URL() string {
	if base.isProd {
		return "https://" + base.domain
	} else {
		return "http://localhost:" + base.port
	}
}

func (base *Base) RenderTempl(c fiber.Ctx, cmp templ.Component, input ...fiber.Map) error {
	c.Set("Content-Type", "text/html")
	return cmp.Render(c.RequestCtx(), c.Response().BodyWriter())
}

func (base *Base) RenderString(c fiber.Ctx, htmlString string) error {
	tmpl, err := template.New("webpage").Funcs(base.Engine.Funcmap).Parse(htmlString)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, nil)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	htmlOutput := buf.String()
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTML)
	return c.SendString(htmlOutput)
}

func (base *Base) GetTemplateString(htmlString string) string {
	tmpl, err := template.New("webpage").Funcs(base.Engine.Funcmap).Parse(htmlString)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, nil)
	if err != nil {
		return ""
	}
	htmlOutput := buf.String()
	return htmlOutput
}

func (base *Base) RenderCached(c fiber.Ctx, templatePath string, input fiber.Map, layouts ...string) error {
	templateKey := templatePath + "-render"
	templateContent := base.Bank.GetString(templateKey)
	if len(templateContent) > 0 {
		// log.Infof("using cache for %s", templatePath)
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTML)
		return c.SendString(templateContent)
	}
	layoutFile := "views/layouts/main"
	if len(layouts) > 0 {
		layoutFile = layouts[0]
	}

	var buf bytes.Buffer
	err := base.Engine.Render(&buf, templatePath, input, layoutFile)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	// log.Infof("rendering %s", templatePath)
	htmlOutput := buf.String()
	base.Bank.SetString(templateKey, htmlOutput, 30*time.Second)
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTML)
	return c.SendString(htmlOutput)
}

func (base Base) Serve(app *fiber.App) {
	app.Get("/sitemap.xml", func(c fiber.Ctx) error {
		return base.SiteMap.Get(c)
	})

	app.Get("/qr-code", func(c fiber.Ctx) error {
		return base.QR.Send(c, base.URL())
	})

	app.Get("/image", func(c fiber.Ctx) error {
		finalPath := c.Query("path")
		return c.SendString("<img alt='" + finalPath + "' style='opacity:0' onload='this.style.opacity=1' class='gen-image' src='" + finalPath + "' width=100%>")
	})

	app.Get("/cmp/:hash", func(c fiber.Ctx) error {
		hash := c.Params("hash")
		templateKey := "cmp-" + hash

		templateContent := base.Bank.GetString(templateKey)
		if len(templateContent) > 0 {
			// log.Infof("using cache for %s", templateKey)
			base.Flash.KeepCached(c, 60*60*24*365)
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTML)
			return c.SendString(templateContent)
		}
		// log.Infof("rendering %s", templateKey)
		htmlContent := base.GetTemplateString(base.Shelf.Get(templateKey))
		if htmlContent != "" {
			base.Bank.SetString(templateKey, htmlContent, 3*time.Minute)
		}
		return base.RenderString(c, htmlContent)
	})

	go func() {
		if err := app.Listen(base.Anchor, fiber.ListenConfig{EnablePrefork: true}); err != nil {
			log.Panic(err)
		}
	}()

	// create channel to signify a signal being sent
	c := make(chan os.Signal, 1)

	// when an interrupt or termination signal is sent, notify the channel
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// block the main thread until an interrupt is received
	_ = <-c
	log.Info("gracefully shutting down...")
	_ = app.Shutdown()

	// cleanup tasks
	log.Info("running cleanup tasks...")
	if base.DB != nil {
		if err := base.DB.Close(); err != nil {
			log.Errorf("failed to close database connection: %v", err)
		}
	}
	if base.WaitGroup != nil {
		base.WaitGroup.Wait()
	}

	base.Bank.Close()
	close(*base.ImageChannel)

	log.Info("fiber app was successfully shutdown.")
}

func showElapsed(description string, start time.Time) {
	if !fiber.IsChild() {
		elapsed := time.Since(start)
		log.Infof("%s: %v\n", description, elapsed)
	}
}

func NewApp(config AppConfig) (*fiber.App, Base) {
	if config.User == "" {
		fmt.Println("config error: user not specified e.g. john")
		os.Exit(1)
	}
	if config.IP == "" {
		fmt.Println("config error: IP not specified e.g. example.com")
		os.Exit(1)
	}
	if config.Port == "" {
		fmt.Println("config error: port not specified e.g. 9910")
		os.Exit(1)
	}
	if config.AppName == "" {
		fmt.Println("config error: app name not specified e.g. myapp")
		os.Exit(1)
	}

	start := time.Now()
	gob.Register(map[string]string{})
	gob.Register(models.User{})

	// declare database URIs
	var dbURI string = os.Getenv("FIBER_USER_URI")

	// init database connection
	db, err := helpers.OpenDB(dbURI + config.AppName + "?parseTime=true&multiStatements=true")
	if err != nil {
		log.Fatal(err)
		return nil, Base{}
	}

	// run special migrations
	helpers.InitShelf(db, config.AppName)
	models.InitUsers(db, config.AppName)

	shelf := &helpers.ShelfModel{DB: db}

	// init storage middleware
	storage := valkey.New(valkey.Config{
		InitAddress: []string{"localhost:6379"},
		Username:    "",
		Password:    "",
		SelectDB:    0,
		Reset:       false,
		TLSConfig:   nil,
	})

	// init bank model
	bank := helpers.NewBank(storage, config.AppName)

	fingerprints := make(map[string]string, 50)
	optimizations := make(map[string]string, 50)

	// generate new minified style file with fingerprint in file name
	helpers.GenerateFingerprintsForFolder("static", "static/gen", ".css", &fingerprints)

	// optimize css files for used class names
	err = helpers.SaveCSSClasses(config.Templates, "static/gen/mango-opt.css",
		"static/styles/mango-tokens.css", "static/styles/mango-utils.css", "static/styles/mango-blocks.css")
	if err != nil {
		log.Errorf("failed to crunch CSS: %v", err)
	}

	// save components into shelf
	cmpLog := map[string]string{}
	err = helpers.SaveComponents(config.Templates, shelf, bank, &cmpLog)

	// combine stylesheet files into a single file and fingerprint
	helpers.CombineAndFingerprint("static/gen/mango-final.css", &fingerprints,
		"static/styles/mango.css", "static/styles/mango-tokens.css", "static/styles/mango-utils.css", "static/styles/mango-blocks.css")
	helpers.CombineAndFingerprint("static/gen/mango-simplified.css", &fingerprints,
		"static/styles/mango.css", "static/gen/mango-opt.css")
	helpers.CombineAndFingerprint("static/gen/grug.css", &fingerprints,
		"static/styles/grug.css", "static/styles/grug-utils.css", "static/styles/grug-tokens.css", "static/styles/grug-blocks.css")

	showElapsed("app resource optimization time", start)

	// get core directory
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Error("failed to get caller information")
		return nil, Base{}
	}
	coreDir, err := filepath.Abs(filename)
	if err != nil {
		log.Fatalf("failed to get absolute path: %v", err)
		return nil, Base{}
	}

	if !fiber.IsChild() {
		// create remote directory for adding migration scripts
		helpers.CreateDirectory("remote/")

		// create uploads directory for uploads via forms
		helpers.CreateDirectory("uploads/")
	}

	if !fiber.IsChild() {
		// create Makefile, gitignore and service files for deployment on remote machine
		helpers.FileSubstitute(filepath.Dir(coreDir)+"/Makefile", "Makefile.example", map[string]string{
			"user":    config.User,
			"appName": config.AppName,
			"ip":      config.IP,
			"port":    config.Port,
		})
		helpers.FileSubstitute(filepath.Dir(coreDir)+"/gitignore.example", ".gitignore.example", map[string]string{
			"user":    config.User,
			"appName": config.AppName,
			"ip":      config.IP,
			"port":    config.Port,
		})
		helpers.FileSubstitute(filepath.Dir(coreDir)+"/example.nginx", fmt.Sprintf("remote/%s.nginx", config.IP), map[string]string{
			"user":    config.User,
			"appName": config.AppName,
			"ip":      config.IP,
			"port":    config.Port,
		})
		helpers.FileSubstitute(filepath.Dir(coreDir)+"/example.service", fmt.Sprintf("remote/%s.service", config.AppName), map[string]string{
			"user":    config.User,
			"appName": config.AppName,
			"ip":      config.IP,
		})
		helpers.FileSubstitute(filepath.Dir(coreDir)+"/air/.air.toml", ".air.toml.example", map[string]string{
			"port": config.Port,
		})

		if !helpers.FileExists("config.env") {
			helpers.FileSubstitute(filepath.Dir(coreDir)+"/air/config.env", "config.env", map[string]string{})
		}

		if !helpers.FileExists("static/main.css") {
			helpers.TouchFile("static/main.css")
		}
	}

	if !fiber.IsChild() {
		helpers.SaveTextToDirectory(strings.ReplaceAll(`
CREATE DATABASE IF NOT EXISTS <appName>;
GRANT ALL PRIVILEGES ON <appName>.* TO 'fiber_user'@'localhost';
FLUSH PRIVILEGES;

-- Verify permissions
SHOW GRANTS FOR 'fiber_user'@'localhost';
	`, "<appName>", config.AppName), "remote/create_app_database.sql")

		helpers.SaveTextToDirectory(`
	-- Create fiber user
CREATE USER IF NOT EXISTS 'fiber_user'@'localhost' IDENTIFIED BY 'USER_PWD';

-- Create fiber database
CREATE DATABASE IF NOT EXISTS fiber;
USE fiber;

-- Grant privileges to the fiber user
GRANT ALL PRIVILEGES ON fiber.* TO 'fiber_user'@'localhost';
FLUSH PRIVILEGES;

	`, "remote/create_fiber_user.sql")

		helpers.SaveTextToDirectory(`
			read -p "Enter password for user: " DB_PASSWORD
echo "Setting environment variable FIBER_USER_URI"
grep -q FIBER_USER_URI /etc/environment || echo "FIBER_USER_URI='fiber_user:${DB_PASSWORD}@tcp(localhost:3306)/'" | sudo tee -a /etc/environment
grep -q FIBER_USER_URI ~/.bashrc || echo "export FIBER_USER_URI='fiber_user:${DB_PASSWORD}@tcp(localhost:3306)/'" | sudo tee -a ~/.bashrc
cat ./remote/create_fiber_user.sql | sed "s/USER_PWD/$DB_PASSWORD/g" | sudo mysql
exec bash

			`, "remote/create_fiber_user.sh")
	}

	if !fiber.IsChild() {
		helpers.CreateDirectory("views/layouts")
		helpers.CreateDirectory("views/partials")
		helpers.CreateDirectory("static/styles")
		helpers.CreateDirectory("static/gen/img")
		helpers.CreateDirectory("static/img")
		helpers.CreateDirectory("static/script")
	}

	showElapsed("app directory creation time", start)

	if !fiber.IsChild() {
		// copy partials from core
		helpers.CopyDir(filepath.Dir(coreDir)+"/partials/", "views/partials/", false)

		// copy images and scripts from core, skipping any repeats
		helpers.CopyDir(filepath.Dir(coreDir)+"/script/", "static/script/", true)
		helpers.CopyDir(filepath.Dir(coreDir)+"/img/", "static/img/", true)

		// copy styles from core
		helpers.CopyDir(filepath.Dir(coreDir)+"/styles/", "static/styles/", false)
	}

	showElapsed("app resource copy time", start)

	imageChannel := make(chan *helpers.SafeImage, 10)

	var wg sync.WaitGroup

	go func() {
		for si := range imageChannel {
			go func() {
				// log.Infof("received safe image from image channel: %v", si)
				si.ProcessImage(start)
			}()
		}
	}()

	if !fiber.IsChild() {
		// generate favicon
		helpers.ConvertJPGToPNG("static/img/favicon.jpg", "static/img/favicon.png")
		helpers.GenerateFavicon("static/img/favicon.png", "static/gen/img/")

		// convert images to Webp
		// helpers.ConvertInlineWebpFolder(&imageChannel, "static/img/", ".jpg", ".png","jpeg")
	}
	showElapsed("app favicon generation time", start)

	// create template engine
	engine := html.New("./views", ".html")
	if config.Templates != nil {
		engine = html.NewFileSystem(http.FS(*config.Templates), ".html")
	}

	// register presets
	formPresets := helpers.FormPresets()
	externalPresets := helpers.ExternalPresets()

	// add functions to template engine
	engine.AddFuncMap(map[string]interface{}{
		"whatsapp": func(phoneNumber string, message ...string) ht.HTML {
			phoneNumberStripped := strings.ReplaceAll(phoneNumber, " ", "")
			phoneNumberStripped = strings.ReplaceAll(phoneNumberStripped, "-", "")
			phoneNumberStripped = strings.ReplaceAll(phoneNumberStripped, "+", "")
			if len(message) > 0 {
				link := fmt.Sprintf("<a href='https://wa.me/%s?text=%s'>%s</a>", phoneNumberStripped, phoneNumber, url.QueryEscape(message[0]))
				return ht.HTML(link)
			}
			link := fmt.Sprintf("<a href='https://wa.me/%s'>%s</a>", phoneNumberStripped, phoneNumber)
			return ht.HTML(link)
		},
		"tel": func(phoneNumber string, message ...string) ht.HTML {
			phoneNumberStripped := strings.ReplaceAll(phoneNumber, " ", "")
			phoneNumberStripped = strings.ReplaceAll(phoneNumberStripped, "-", "")
			link := fmt.Sprintf("<a href='tel:%s'>%s</a>", phoneNumberStripped, phoneNumber)
			return ht.HTML(link)
		},
		"humanDate": func(t time.Time) string {
			return t.UTC().Format("Jan 02, 2006")
		},
		"humanTime": func(t time.Time) string {
			return t.UTC().Format("Jan 02, 2006 @ 15:04 hrs")
		},
		"humanYear": func(t time.Time) string {
			return t.UTC().Format("2006")
		},
		"rev": func(classes ...string) ht.HTMLAttr {
			if len(classes) > 0 {
				return ht.HTMLAttr("class='rev " + classes[0] + "' hx-trigger='revealed'")
			}
			return ht.HTMLAttr("class='rev' hx-trigger='revealed'")
		},
		"intersect": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, imgPath, "static/gen/img", dimensions...)
			return ht.HTML(`
<img alt="` + outputPath + `" class="rev-image" hx-trigger="revealed" src="` + outputPath + `">
			`)
		},
		"intersects": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, "static/img/"+imgPath, "static/gen/img", dimensions...)
			return ht.HTML(`
<img alt="` + outputPath + `" class="rev-image" hx-trigger="revealed" src="` + outputPath + `">
			`)
		},
		"gfont": func(fontName string, selector string) ht.HTML {
			return ht.HTML(`
			<link rel="preconnect" href="https://fonts.googleapis.com">
			<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
			<link href="https://fonts.googleapis.com/css2?family=` + strings.ReplaceAll(fontName, " ", "+") + `&display=swap" rel="stylesheet" media="print" onload="this.media='all'">
				<style>
				` + selector + `{
					font-family: "` + fontName + `", sans-serif;
				}
				</style>`)
		},
		"gfonts": func(args ...string) ht.HTML {
			if len(args)%2 != 0 {
				return ht.HTML("<!-- (gfonts) Please specify font-selectors pairs -->")
			}
			selectorsQueue := `<style>`
			start := `
			<link rel="preconnect" href="https://fonts.googleapis.com">
			<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>

			<link href="https://fonts.googleapis.com/css2`
			for i := 0; i < len(args); i += 2 {
				fontName := args[i]
				selectors := args[i+1]
				if i == 0 {
					start += "?family="
				} else {
					start += "&family="
				}
				selectorsQueue += selectors + `{ font-family: "` + fontName + `",sans-serif; } `
				start += strings.ReplaceAll(fontName, " ", "+")
			}

			end := `&display=swap&text=ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz1234567890" rel="stylesheet" media="print" onload="this.media='all'">
				`
			selectorsQueue += `</style>`
			start += end
			start += selectorsQueue
			return ht.HTML(start)
		},
		"role": func(roles interface{}, role string) bool {
			if roles == nil {
				return false
			}
			return strings.Contains(roles.(string), "|"+role+"|")
		},
		"default": func(def string, value interface{}) interface{} {
			if value == nil {
				return def
			}
			return value
		},
		"prod": func() bool {
			return config.IsProduction
		},
		"svg": func(iconName string) ht.HTML {
			return ht.HTML(`
			<script
    class="script-tag"
    data-svg-src="/static/img/bootstrap-icons/` + iconName + `.svg"
    hx-get="/static/img/bootstrap-icons/` + iconName + `.svg"
    hx-swap="outerHTML"
    hx-trigger="load">
</script>
			`)
		},
		"gen": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, imgPath, "static/gen/img", dimensions...)
			return ht.HTML(outputPath)
		},
		"gens": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, "static/img/"+imgPath, "static/gen/img", dimensions...)
			return ht.HTML(outputPath)
		},
		"preload": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, imgPath, "static/gen/img", dimensions...)
			return ht.HTML("<link rel='preload' href='" + outputPath + "' as='image' fetchpriority='high'>")
		},
		"preloads": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, "static/img/"+imgPath, "static/gen/img", dimensions...)
			return ht.HTML("<link rel='preload' href='" + outputPath + "' as='image' fetchpriority='high'>")
		},
		"him": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, imgPath, "static/gen/img", dimensions...)
			htmxString := `<div class="full-w" hx-get="/image?path=` + outputPath + `" hx-trigger="revealed" hx-swap="outerHTML">
				            </div>`
			return ht.HTML(htmxString)
		},
		"hims": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, "static/img/"+imgPath, "static/gen/img", dimensions...)
			htmxString := `<div class="full-w" hx-get="/image?path=` + outputPath + `" hx-trigger="revealed" hx-swap="outerHTML">
				            </div>`
			return ht.HTML(htmxString)
		},
		"lazy": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, imgPath, "static/gen/img", dimensions...)
			return ht.HTML("<img loading='lazy' decode='async' alt='" + outputPath + "' style='opacity:0' onload='this.style.opacity=1' class='gen-image' src='" + outputPath + "'>")
		},
		"lazys": func(imgPath string, dimensions ...int) ht.HTML {
			outputPath := "/" + helpers.ConvertInlineWebp(&imageChannel, "static/img/"+imgPath, "static/gen/img", dimensions...)
			return ht.HTML("<img loading='lazy' decode='async' alt='" + outputPath + "' style='opacity:0' onload='this.style.opacity=1' class='gen-image' src='" + outputPath + "'>")
		},
		"icon": func(iconName ...string) ht.HTML {
			width := "20px"
			height := "20px"
			if len(iconName) > 1 {
				width = iconName[1]
				height = iconName[1]
			}
			if len(iconName) > 2 {
				height = iconName[2]
			}
			return ht.HTML(`
			<div style="width:` + width + `;height:` + height + `;display:flex;align-items:center;justify-content:center;">
			<script
    class="script-tag"
    data-svg-src="/static/img/bootstrap-icons/` + iconName[0] + `.svg"
    hx-get="/static/img/bootstrap-icons/` + iconName[0] + `.svg"
    hx-swap="outerHTML"
    hx-trigger="load">
</script>
</div>
			`)
		},
		"ct": func() time.Time {
			return time.Now()
		},
		"input": func(key string) ht.HTML {
			return ht.HTML(formPresets[key])
		},
		"preset": func(key string) ht.HTML {
			return ht.HTML(externalPresets[key])
		},
		"extern": func(key string) ht.HTML {
			return ht.HTML(externalPresets[key])
		},
		"minify": func(s string) string {
			return "/" + fingerprints[s]
		},
		"min": func(s string) string {
			return "/" + fingerprints[s]
		},
		"optimize": func(s string) string {
			return "/" + optimizations[s]
		},
		"opt": func(s string) string {
			return "/" + optimizations[s]
		},
		"toUpper": func(s string) string {
			return strings.ToUpper(s)
		},
		"toLower": func(s string) string {
			return strings.ToLower(s)
		},
		"in": func(outer string, inner string) bool {
			return strings.Contains(outer, inner)
		},
		"add": func(a, b int) int {
			return a + b
		},
		"mod": func(a, b int) int {
			return a % b
		},
		"slice": func(a ...any) []any {
			return a
		},
		"split": func(str, delim string) []string {
			return strings.Split(str, delim)
		},
		"replace": func(str, before, after string) string {
			return strings.ReplaceAll(str, before, after)
		},
		"trimPrefix": func(str, prefix string) string {
			return strings.TrimPrefix(str, prefix)
		},
		"trimSuffix": func(str, suffix string) string {
			return strings.TrimSuffix(str, suffix)
		},
		"trimSpace": func(str string) string {
			return strings.TrimSpace(str)
		},
		"Condense": func(str string) string {
			return helpers.ReplaceSpecial(str)
		},
		"condense": func(str string) string {
			return helpers.ReplaceSpecial(str)
		},
		"Get": func(key string) string {
			val, exists := (*config.SiteInfo)[key]
			if exists {
				return val
			}
			return "<" + key + ">"
		},
		"get": func(key string) string {
			val, exists := (*config.SiteInfo)[key]
			if exists {
				return val
			}
			return "<" + key + ">"
		},
		"mimeType": func(name string) string {
			ext := filepath.Ext(name)
			return mime.TypeByExtension(ext)
		},
		"use": func(values []fiber.OldInputData, key string) string {
			for _, old := range values {
				if old.Key == key {
					return old.Value
				}
			}
			return ""
		},
		"safeHTML": func(s string) ht.HTML {
			return ht.HTML(s)
		},
		"eq": func(s1, s2 any) bool {
			return s1 == s2
		},
		"favicon": func() ht.HTML {
			links := `
		<link rel="apple-touch-icon" sizes="180x180" href="/static/gen/img/apple-touch-icon.png?v=(())">
		<link rel="icon" type="image/png" sizes="32x32" href="/static/gen/img/favicon-32x32.png?v=(())">
		<link rel="icon" type="image/png" sizes="16x16" href="/static/gen/img/favicon-16x16.png?v=(())">
		<link rel="manifest" href="/static/gen/img/site.webmanifest?v=(())">
			`
			return ht.HTML(strings.ReplaceAll(links, "(())", optimizations["img/favicon.png"]))
		},
		"cmp": func(name string, snippet ...ht.HTML) ht.HTML {
			return ht.HTML(`<div hx-get="/cmp/` + cmpLog[name] + `" hx-trigger="load" hx-target="this" hx-swap="outerHTML"></div>`)
		},
	})

	// add other func map
	if config.FuncMap != nil {
		engine.AddFuncMap(config.FuncMap)
	}

	if err := engine.Load(); err != nil {
		log.Errorf("failed to load templates: %v", err)
		return nil, Base{}
	}

	showElapsed("template engine load time", start)

	// fiber specific configuration

	// create new fiber prefork app
	app := fiber.New(fiber.Config{
		AppName:           config.AppName,
		Views:             engine,
		ViewsLayout:       "views/layouts/main",
		PassLocalsToViews: true,
		CaseSensitive:     true,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReduceMemoryUsage: config.ReduceMemoryUsage,
	})

	// init fiber session middleware
	// sessEx = extractors.FromCookie("__Host-session_id_")
	sessEx := extractors.FromCookie("__Host-session_id_local_" + config.AppName + "_")
	if config.IsProduction {
		sessEx = extractors.FromCookie("__Host-session_id_" + config.AppName + "_")
	}

	sessionIdleTimeout := time.Minute * 5
	sessionAbsoluteTimeout := time.Hour * 3
	if config.SessionAbsoluteTimeout != 0 {
		sessionAbsoluteTimeout = config.SessionAbsoluteTimeout
	}
	if config.SessionIdleTimeout != 0 && config.SessionIdleTimeout < sessionAbsoluteTimeout {
		sessionIdleTimeout = config.SessionIdleTimeout
	}
	sessConfig := session.Config{
		IdleTimeout:     sessionIdleTimeout,
		AbsoluteTimeout: sessionAbsoluteTimeout,
		CookieSecure:    true,
		CookieHTTPOnly:  true,
		CookieSameSite:  "Lax",
		Storage:         storage,
		Extractor:       sessEx,
	}
	sessionStore := session.NewStore(sessConfig)
	sessionStore.RegisterType(models.User{})
	sessionStore.RegisterType(map[string]string{})

	// create csrf error handler
	csrfErrorHandler := func(c fiber.Ctx, err error) error {
		log.Errorf("csrf error: %v request: %v from IP: %v", err, c.OriginalURL(), c.IP())
		switch c.Accepts("html", "json") {
		case "json":
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "403 Forbidden",
			})
		default:
			return c.Redirect().With("csrf", fmt.Sprintf("CSRF Error: %v", err)).Back()
		}
	}

	// init fiber csrf middleware
	csrfMiddleware := csrf.New(csrf.Config{
		CookieName:        "__Host-csrf_",
		CookieSecure:      true,
		CookieHTTPOnly:    false,
		CookieSameSite:    "Lax",
		CookieSessionOnly: false,
		Extractor: extractors.Chain(
			extractors.FromForm("csrf"),
		),
		Session:        sessionStore,
		ErrorHandler:   csrfErrorHandler,
		Storage:        storage,
		SingleUseToken: true,
		TrustedOrigins: []string{
			"http://127.0.0.1",
			"http://127.0.0.1:" + config.Port,
			"http://127.0.0.1:8081",
			"http://localhost",
			"http://localhost:" + config.Port,
			"http://localhost:8081",
			"https://" + config.IP,
		},
	})

	app.Use(csrfMiddleware)

	// init fiber logger format
	app.Use(logger.New(logger.Config{
		Format: "[${ip}]:${port} ${latency} -> ${status} - ${method} ${path}\n",
	}))
	f, err := os.OpenFile(config.AppName+".log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	iw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(iw)

	// init static file serving
	app.Get("/static/*", static.New("./static", static.Config{
		Compress:      false,
		ByteRange:     true,
		Browse:        true,
		IndexNames:    []string{"index.html"},
		CacheDuration: 31536000 * time.Second,
		MaxAge:        31536000,
	}))

	if config.Templates == nil {
		if !helpers.FileExists("views/index.html") {
			helpers.TouchFile("views/index.html")
		}
	}

	// init email model
	mailModel := email.NewMailModel(db, &wg, config.AppName)

	// init base
	base := Base{
		Users:        models.NewUserModel(db, sessionStore),
		DB:           db,
		Store:        sessionStore,
		Shelf:        shelf,
		Flash:        &helpers.FlashModel{Store: sessionStore},
		Files:        &helpers.FilesModel{},
		Bank:         bank,
		MMG:          payments.NewMMG(db, bank, &wg, config.AppName),
		Anchor:       ":" + config.Port,
		Engine:       engine,
		QR:           helpers.NewQR(),
		Mail:         mailModel,
		WaitGroup:    &wg,
		SiteMap:      helpers.NewSitemap(config.IP),
		ImageChannel: &imageChannel,

		isProd: config.IsProduction,
		domain: config.IP,
		port:   config.Port,
	}

	app.Use(etag.New(etag.Config{
		Weak: false,
	}))

	if config.IsProduction {
		app.Use(recover.New())
	}
	app.Use(idempotency.New(idempotency.Config{
		Storage: storage,
	}))

	app.Use(pprof.New(pprof.Config{Prefix: "/profiler"}))

	app.Use(helpers.IncludeSessionLocals(sessionStore))

	environment := "dev"
	if config.IsProduction {
		environment = "prod"
	}
	if !fiber.IsChild() {
		elapsed := time.Since(start)
		log.Infof("(%s - RM: %v) app startup time: %v\n", environment, config.ReduceMemoryUsage, elapsed)
	}

	// return configured fiber app and base
	return app, base
}
