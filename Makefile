# Copyright 2021 NetApp, Inc.  All Rights Reserved
.DEFAULT_GOAL:=help

.PHONY: help deps clean build package

###############################################################################
# Anything that needs to be done before we build everything
#  Check for GCC, GO version, etc and anything else we are dependent on.
###############################################################################
GCC_EXISTS := $(shell which gcc)
REQUIRED_GO_VERSION := 1.15
ifneq (, $(shell which go))
FOUND_GO_VERSION := $(shell go version | cut -d" " -f3 | cut -d"o" -f 2)
CORRECT_GO_VERSION := $(shell expr `go version | cut -d" " -f3 | cut -d"o" -f 2` \>= ${REQUIRED_GO_VERSION})
endif
RELEASE      ?= $(shell git describe --tags --abbrev=0)
VERSION      ?= $(shell expr `date +%Y.%m.%d%H | cut -c 3-`)
COMMIT       := $(shell git rev-parse --short HEAD)
BUILD_DATE   := `date +%FT%T%z`
LD_FLAGS     := "-X 'goharvest2/cmd/harvest/version.VERSION=$(VERSION)' -X 'goharvest2/cmd/harvest/version.Release=$(RELEASE)' -X 'goharvest2/cmd/harvest/version.Commit=$(COMMIT)' -X 'goharvest2/cmd/harvest/version.BuildDate=$(BUILD_DATE)'"
GOARCH ?= amd64
GOOS ?= linux
COLLECTORS := $(shell ls cmd/collectors)
EXPORTERS := $(shell ls cmd/exporters)
HARVEST_RELEASE := harvest-${VERSION}-${RELEASE}
DIST := dist
TMP := /tmp/${HARVEST_RELEASE}

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

header:
	@echo "    _  _                     _     ___   __   "
	@echo "   | || |__ _ _ ___ _____ __| |_  |_  ) /  \  "
	@echo "   | __ / _\` | '_\ V / -_|_-<  _|  / / | () | "
	@echo "   |_||_\__,_|_|  \_/\___/__/\__| /___(_)__/  "
	@echo

deps: header ## Check dependencies
	@echo "checking Harvest dependencies"
ifeq (${GCC_EXISTS}, )
	@echo
	@echo "Harvest requires that you have gcc installed."
	@echo
	@exit 1
endif
	@# Make sure that go exists
ifeq (${FOUND_GO_VERSION}, )
	@echo
	@echo "Harvest requires that the go lang is installed and is at least version: ${REQUIRED_GO_VERSION}"
	@echo
	@exit 1
endif
	@# Check to make sure that GO is the correct version
ifeq ("${CORRECT_GO_VERSION}", "0")
	@echo
	@echo "Required go lang version is ${REQUIRED_GO_VERSION}, but found ${FOUND_GO_VERSION}"
	@echo
	@exit 1
endif

clean: header ## Cleanup the project binary (bin) folders
	@echo "Cleaning harvest files"
	@rm -rf bin

build: clean deps harvest collectors exporters ## Build the project

packages: clean deps build dist-tar ## Package Harvest binary

all: packages ## Build and Package

harvest: deps
	@# Build the harvest cli
	@echo "Building harvest"
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/harvest -ldflags=$(LD_FLAGS) cmd/harvest/harvest.go

	@# Build the harvest poller
	@echo "Building poller"
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/poller -ldflags=$(LD_FLAGS) cmd/poller/poller.go

	@# Build the daemonizer for the pollers
	@echo "Building daemonizer"
	@cd cmd/tools/daemonize; gcc daemonize.c -o ../../../bin/daemonize

	@# Build the zapi tool
	@echo "Building zapi tool"
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/zapi -ldflags=$(LD_FLAGS) cmd/tools/zapi/main/main.go

	@# Build the grafana tool
	@echo "Building grafana tool"
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/grafana -ldflags=$(LD_FLAGS) cmd/tools/grafana/main/main.go

###############################################################################
# Collectors
###############################################################################
collectors: deps
	@echo "Building collectors:"
	@for collector in ${COLLECTORS}; do                                                   \
		cd cmd/collectors/$${collector};                                              \
		echo "  Building $${collector}";                                              \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags=$(LD_FLAGS) -buildmode=plugin -o ../../../bin/collectors/"$${collector}".so;     \
		if [ -d plugins ]; then                                                       \
			echo "    Building plugins for $${collector}";                        \
	        	cd plugins;                                                           \
	        	for plugin in `ls`; do                                                \
				echo "        Building: $${plugin}";                          \
				cd $${plugin};                                                \
				GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags=$(LD_FLAGS) -buildmode=plugin -o ../../../../../bin/plugins/"$${collector}"/"$${plugin}".so; \
				cd ../;                                                       \
			done;                                                                 \
			cd ../../../../;                                                      \
		else                                                                          \
	       		cd - > /dev/null;                                                     \
		fi;                                                                           \
	done

###############################################################################
# Exporters
###############################################################################
exporters: deps
	@echo "Building exporters:"
	@for exporter in ${EXPORTERS}; do                                                     \
		cd cmd/exporters/$${exporter};                                                \
		echo "  Building $${exporter}";                                               \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags=$(LD_FLAGS) -buildmode=plugin -o ../../../bin/exporters/"$${exporter}".so;       \
	       	cd - > /dev/null;                                                             \
	done



###############################################################################
# Build tar gz distribution
###############################################################################
dist-tar:
	@echo "Building tar"
	@rm -rf ${TMP}
	@rm -rf ${DIST}
	@mkdir ${TMP}
	@mkdir ${DIST}
	@cp -a bin conf docs grafana README.md LICENSE ${TMP}
	@cp -a harvest.example.yml ${TMP}/harvest.yml
	@tar --directory /tmp --create --gzip --file ${DIST}/${HARVEST_RELEASE}.tar.gz ${HARVEST_RELEASE}
	@rm -rf ${TMP}
	@echo "tar artifact @" ${DIST}/${HARVEST_RELEASE}.tar.gz
