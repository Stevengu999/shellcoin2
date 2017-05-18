#!/usr/bin/env bash

go build
mv cli $GOPATH/bin/shellcoin-cli
echo "install success!"