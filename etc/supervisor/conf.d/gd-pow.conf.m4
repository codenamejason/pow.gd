[program:gd-pow]
directory = /home/chilts/src/appsattic-pow.gd
command = /home/chilts/src/appsattic-pow.gd/bin/pow
user = chilts
autostart = true
autorestart = true
stdout_logfile = /var/log/chilts/gd-pow-stdout.log
stderr_logfile = /var/log/chilts/gd-pow-stderr.log
environment =
    POW_PORT="__POW_PORT__",
    POW_BASE_URL="__POW_BASE_URL__",
    POW_NAKED_DOMAIN="__POW_NAKED_DOMAIN__"
