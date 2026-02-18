production_host_ip = "<ip>"
project_name = "<appName>"
username = "<user>"

YELLOW=\033[33m
NC=\033[0m

## help: list all options
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## status: see the status of the remote app
.PHONY: status
status:
	@ssh ${username}@${production_host_ip} "service <appName> status"

## deploy: upload updated version of app to VPS
.PHONY: deploy
deploy: deploy/static deploy/app

## deploy/db: sign into VPS and set up database
.PHONY: deploy/db
deploy/db:
	@-figlet "Deploy db"
	@echo "• deploying database on VPS"
	@ssh -t ${username}@${production_host_ip} "cd ~/${project_name}/ && sudo make up"

## deploy/nginx: sign into VPS and set up nginx config
.PHONY: deploy/nginx
deploy/nginx:
	@-figlet "Deploy nginx"
	@echo "• setting up nginx config on VPS"
	@ssh -t ${username}@${production_host_ip} "cd ~/${project_name}/ && sudo make nginx"

## deploy/first: upload first version of app to VPS
.PHONY: deploy/first
deploy/first: deploy/folder deploy/static deploy/db deploy/app
	@echo "• installed app on VPS"

## deploy/app: upload only app
.PHONY: deploy/app
deploy/app: build
	@-figlet "Deploy app"
	@echo "• transferring binary to VPS"
	@echo "${YELLOW}"
	@rsync -cP ./bin/linux_amd64/web ${username}@${production_host_ip}:~/${project_name}/
	@echo "${NC}"
	@-figlet "Restart app"
	@echo "• restarting app service"
	@ssh -t ${username}@${production_host_ip} "cd ~/${project_name}/ && sudo systemctl link remote/${project_name}.service && sudo systemctl daemon-reload && sudo systemctl enable ${project_name}.service && sudo systemctl restart ${project_name}.service"

## deploy/folder: create app folder
.PHONY: deploy/folder
deploy/folder:
	@-figlet "Deploy folder"
	@echo "• setting up project folder on VPS"
	@ssh -t ${username}@${production_host_ip} "mkdir -p ~/${project_name}/static && sudo gpasswd -a www-data ${username} && sudo chmod g+x ~/${project_name}/static && chmod 700 ~/.ssh && chmod 600 ~/.ssh/authorized_keys"

## deploy/static: upload only static files
.PHONY: deploy/static
deploy/static:
	@-figlet "Deploy static"
	@echo "• transferring static files to VPS"
	@echo "${YELLOW}"
	@rsync -uzrP --exclude='static/gen' ./static ./remote ./Makefile ./config.env ${username}@${production_host_ip}:~/${project_name}/
	@echo "${NC}"

## build: build local copy
.PHONY: build
build:
	@-figlet "Build app"
	@echo "• building binary locally"
	@go build -ldflags='-s' -o=./bin/web .
	@GOOS=linux GOARCH=amd64 go build -ldflags='-s' -o=./bin/linux_amd64/web .

## tidy: tidy module dependencies and format all .go files
.PHONY: tidy
tidy:
	@-figlet "Tidy"
	@echo '• tidying module dependencies...'
	go mod tidy
	@echo '• verifying and vendoring module dependencies...'
	go mod verify
	go mod vendor
	@echo '• formatting .go files...'
	go fmt ./...

## audit: run quality control checks
.PHONY: audit
audit:
	@-figlet "Audit"
	@echo '• checking module dependencies...'
	go mod tidy -diff
	go mod verify
	@echo '• vetting code...'
	go vet ./...
	go tool staticcheck ./...
	@echo '• running tests...'
	go test -race -vet=off ./...

## user: create fiber user
.PHONY: user
user:
	@-figlet "Create user"
	sh ./remote/create_fiber_user.sh

## up: create app database
.PHONY: up
up:
	@-figlet "Migrate up"
	cat remote/create_app_database.sql | mysql

## nginx: move nginx config into sites-enabled directory
.PHONY: nginx
nginx:
	@-figlet "Set up nginx"
	sudo cp remote/<ip>.nginx /etc/nginx/sites-enabled/
	sudo certbot --nginx -d <ip>
	sudo /etc/init.d/nginx reload

## prof: profile app using pprof tool
.PHONY: prof
prof:
	@-figlet "Prof"
	go tool pprof -inuse_space http://localhost:<port>/profiler/debug/pprof/heap

## prof/png: output png of pprof
.PHONY: prof/png
prof/png:
	@-figlet "Prof/png"
	@-go tool pprof -inuse_space -png http://localhost:<port>/profiler/debug/pprof/heap > static/gen/prof.png

