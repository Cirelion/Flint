#!/bin/bash
VERSION=$(git describe --tags)
echo Building version $VERSION
go build -ldflags "-X github.com/cirelion/flint/common.VERSION=${VERSION}"