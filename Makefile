#
# Makefile for generating "redis-x-cli" 
#
# Version: 1.0.0
#

all: 
	go build 

# From: https://www.digitalocean.com/company/blog/get-your-development-team-started-with-go/ 
# dpkg -c redis-x-cli_0.9.0-61_amd64.deb  -- To see files in a .deb

VERSION=1.0.0
BUILD=$(shell git rev-list --count HEAD)

redis-x-cli-dpkg:
	mkdir -p deb/redis-x-cli/usr/local/bin deb/redis-x-cli/usr/local/etc/redis-x-cli deb/redis-x-cli/usr/local/doc
	cp ./redis-x-cli  deb/redis-x-cli/usr/local/bin
	cp ./*.json  deb/redis-x-cli/usr/local/etc/redis-x-cli
	cp ./err.doc  deb/redis-x-cli/usr/local/doc
	fpm -s dir -t deb -n redis-x-cli -v $(VERSION)-$(BUILD) -C deb/redis-x-cli .

redis-x-cli: main.go util_bsd.go util_linux.go util_windows.go
	go build 

install: redis-x-cli
	cp redis-x-cli ~/bin

