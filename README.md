# Boiler
This project is focused on providing boilerplate for a Gofiber app.

## Dependencies
The notable dependencies are:
- Valkey
- MySQL
- libaom-dev
- libwebp-dev

AVIF and WEBP conversions are done using libaom-dev and libwebp-dev:
```sh
sudo apt-get install libaom-dev
sudo apt-get install libwebp-dev
sudo apt install libvips-tools
```

## Basic app
Create a go module and add the following to your **go.mod** file:
```
go 1.26.0

require (
	github.com/gofiber/fiber/v3 v3.0.0
	github.com/joashgobin/boiler-v2 v0.0.35
)

replace github.com/joashgobin/boiler-v2 => ../boiler-v2
```

Run the following:
```sh
go get github.com/gofiber/fiber/v3
go get github.com/joashgobin/boiler-v2
go get github.com/joashgobin/boiler-v2/core
go get github.com/joashgobin/boiler-v2/email
```

Create a *main.go* file and paste the following code:

```go
package main

import (
	"embed"
    "flag"

    "github.com/gofiber/fiber/v3"
	"github.com/joashgobin/boiler-v2/core"
)

//go:embed views/*
var templates embed.FS

type ctx = fiber.Ctx

func main() {
	isProd := flag.Bool("prod", false, "production mode of app (dev vs. prod)")
	flag.Parse()

	config := core.AppConfig{
		User:      "myname",
		IP:        "myapp.example.com",
		Port:      "9911",
		AppName:   "appname",
		Templates: &templates,
		SiteInfo:  &map[string]string{},
		IsProduction: *isProd,
	}
    app, base := core.NewApp(config)

	app.Get("/", func(c ctx) error {
		return c.SendString("Welcome!")
	})

	base.Serve(app)
}
```


Note the following:
- User - the username of the linux user that will be used to log into the VPS the app is being deployed to
- IP - the domain name at which the app will be accessed via the internet when deployed
- Port - the port number the app will run on
- AppName - the name of the app will be used to create the database for the base app
- Templates - the set of view files to be embedded
- SiteInfo - general site information to be accessed in the templates using the "Get" function

We can then embed the view files into the app using go embed:
```sh
mkdir -p views/layouts
mkdir -p views/partials
mkdir -p views/partials/placeholder.html
touch views/layouts/main.html
touch views/index.html
touch views/scripts.html
```

Run the program:
```sh
go run main.go
```

We now need to create the database for our app. Rename the Makefile and run the database migration:
```sh
cp Makefile.example Makefile
cp .gitignore.example .gitignore
cp .air.toml.example .air.toml
sudo make up
```

Re-run the program:
```sh
go run main.go
```

You should use air to run the app:
```
go install github.com/air-verse/air@latest
air
```

If this is your first app using this project as your starter, run the command to create the fiber user:
```sh
sudo make user
```
>MySQL requires a strong password. I recommend {Firstfancyword}{Secondfancyword}__123 as a template

## Deployment to VPS
Upload the first version of the app to the VPS:
```sh
make deploy/first
```
This will upload the static assets, deploy the database, build and upload the binary, create and run the app service

Next we can deploy the nginx configuration and certbot certification with the target:
```sh
make deploy/nginx
```

For successive deployments you will only have to run:
```sh
make deploy
```

To specifically deploy the static assets or the app binary run the following respectively:
```sh
make deploy/static
make deploy/app
```

## Features
- Favicon generation
- Image optimization with fingerprinting
- HTML file prefetching
- Route-specific cache control
- CSS minification with fingerprinting
- Efficient caching
- Deployments to VPS:
    - Database setup
    - Static file transfer
    - Binary transfer and service setup
    - Nginx configuration and certbot certification
- Gzip and Brotli compression of resources via Nginx
- User profile creation and management
- Low memory usage
- Rate limiting
- Caching by ETag
- Sitemap generation
- Panic recovery
- Algorithmic layouts via mango CSS


## Swup JS/HTMX Template
Add the following to your *views/layouts/main.html* file:
```html
<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>My App</title>
    <script>
        function showBody(){
            document.body.classList.add('loaded');
        }
    </script>
    <link rel="preload" as="style" href='{{min "grug.css"}}'>
    <link rel="stylesheet" media="none" onload="this.media='all';showBody()" href='{{min "grug.css"}}'>
    <link rel="stylesheet" media="none" onload="this.media='all'" href='{{min "main.css"}}'>
    <style>
    body {
        opacity: 0;
        transition: opacity 300ms ease-in-out;
    }

    body.loaded{
        opacity: 1;
    }

    :root{
        --prim: white;
        --sec: #001a21;
        --accent: #00adbf;
    }


    </style>

    {{template "views/partials/meta" .}}
    {{template "views/partials/flash-style" .}}
    {{template "views/partials/svg-pop" .}}
    {{template "views/partials/prefetch" .}}
    {{template "views/partials/website-preset" .}}
    {{favicon}}
    {{preset "htmx"}}

</head>

<body class="bb cp">
    <header class="flex cp fixed pad">
        <a href="/" class="grow"><strong>My App</strong></a>
        <nav>
            <ul class="flex right sm">
                {{if .user}}
                <li><a href="/admin/">Dashboard</a></li>
                <form method="post" action="/logout" data-swup-form>
                    <input type="hidden" name="csrf" value="{{.csrf}}">
                    <input class="round ba no-dec" type="submit" name="submit" value="Log out">
                </form>
                {{else}}
                <li><a href="/products/">Products</a></li>
                <li><a href="/about/">About</a></li>
                {{end}}
            </ul>
        </nav>
    </header>
    <main id="swup" class="transition-main">
        {{template "views/partials/flash-body" .}}
        {{embed}}
    </main>
    <footer class="grid gap-2 fit-3 bb cp pad-3"></footer>
    {{template "views/partials/swup" .}}
</body>

</html>


```
