from="/mnt/c/Users/Joash Gobin/Desktop/dev/joashgobin.github.io"
to="core/styles/"

.PHONY: styles
styles:
	cp ${from}/*.css ${to}

.PHONY: update
update:
	go get -u -v ./...
	go mod tidy
	go mod vendor
