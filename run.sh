#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo "shellcoin2 binary dir:" "$DIR"

pushd "$DIR" >/dev/null

go run cmd/shellcoin/shellcoin.go --gui-dir="${DIR}/src/gui/static/" $@

popd >/dev/null
