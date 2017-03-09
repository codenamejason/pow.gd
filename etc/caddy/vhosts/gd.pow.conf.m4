__POW_NAKED_DOMAIN__ {
  proxy / localhost:__POW_PORT__ {
    transparent
  }
  tls chilts@appsattic.com
  log stdout
  errors stderr
}

www.__POW_NAKED_DOMAIN__ {
  redir http://__POW_NAKED_DOMAIN__{uri} 302
}
