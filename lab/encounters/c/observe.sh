#!/bin/sh
set -eu
find "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)" -maxdepth 1 -type f -printf '%f %s bytes\n' | sort
