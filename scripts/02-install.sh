#!/bin/bash
## --------------------------------------------------------------------------------------------------------------------

set -e

echo "Checking ask.sh is installed ..."
if [ ! $(which ask.sh) ]; then
    echo "Please put ask.sh into ~/bin (should already be in your path from ~/.profile):"
    echo ""
    echo "    mkdir ~/bin"
    echo "    wget -O ~/bin/ask.sh https://gist.githubusercontent.com/chilts/6b547307a6717d53e14f7403d58849dd/raw/ecead4db87ad4e7674efac5ab0e7a04845be642c/ask.sh"
    echo "    chmod +x ~/bin/ask.sh"
    echo ""
    exit 2
fi
echo

# General
POW_PORT=`ask.sh pow POW_PORT 'Which local port should the server listen on :'`
POW_NAKED_DOMAIN=`ask.sh pow POW_NAKED_DOMAIN 'What is the naked domain (e.g. localhost:1234 or pow.gd) :'`
POW_BASE_URL=`ask.sh pow POW_BASE_URL 'What is the base URL (e.g. http://localhost:1234 or https://pow.gd) :'`
POW_REDIS_ADDR=`ask.sh pow POW_REDIS_ADDR 'Which Redis server should be used for hits (e.g. ":6379") :'`

echo "Building code ..."
gb build
echo

# copy the supervisor script into place
echo "Copying supervisor config ..."
m4 \
    -D __POW_PORT__=$POW_PORT \
    -D __POW_NAKED_DOMAIN__=$POW_NAKED_DOMAIN \
    -D __POW_BASE_URL__=$POW_BASE_URL \
    -D __POW_REDIS_ADDR__=$POW_REDIS_ADDR \
    etc/supervisor/conf.d/gd-pow.conf.m4 | sudo tee /etc/supervisor/conf.d/gd-pow.conf
echo

# restart supervisor
echo "Restarting supervisor ..."
sudo systemctl restart supervisor.service
echo

# copy the caddy conf
echo "Copying Caddy config config ..."
m4 \
    -D __POW_PORT__=$POW_PORT \
    -D __POW_NAKED_DOMAIN__=$POW_NAKED_DOMAIN \
    -D __POW_BASE_URL__=$POW_BASE_URL \
    -D __POW_REDIS_ADDR__=$POW_REDIS_ADDR \
    etc/caddy/vhosts/gd.pow.conf.m4 | sudo tee /etc/caddy/vhosts/gd.pow.conf
echo

# restarting Caddy
echo "Restarting caddy ..."
sudo systemctl restart caddy.service
echo

## --------------------------------------------------------------------------------------------------------------------
