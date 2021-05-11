#!/bin/bash
set -euo pipefail

echo "Installing tools from go.mod"

tools=$(cat make/tools.go | grep -o '\"[^"]\+\"' | sed 's/"//g')
for tool in $tools
do
    echo "Downloading $tool"
    go get $tool
done
