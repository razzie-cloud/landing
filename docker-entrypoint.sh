#!/bin/sh
set -eu
render-landing --output /srv/index.html
exec "$@"
