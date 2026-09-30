# Desktop frontend placeholder
#
# SIMARC has no JavaScript frontend build. The desktop window is served by the
# Go application itself: cmd/desktop hands every request from the webview to the
# same gin engine cmd/server exposes, so templates, CSS and JS all come from
# web/ (embedded via embed.go) exactly as they do in the browser.
#
# Wails requires this directory to exist, so it stays empty on purpose — there
# is nothing to build here and nothing to keep in sync.
