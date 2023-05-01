# tjenare
Webserver built for my specific needs.

## Not intended for public use!

All I really need is to serve some files based on what subdomain is requested, and maybe some reverse proxying.

No need for all the fancy schmancy configuration options of a real web server when I can get away with a tiny Go project to cover the same needs.

**DO NOT** base any of your own work on this, as it *will* change without notice as my needs change. Fork instead.

I only provide this to others for educational purposes, and do not suggest you will actually use it for serving files publically. For that, I hope it's instructional enough to be of value.

## My needs

The needs I identified before starting the project:
* Serve static files based on what subdomain is requested.
* Minimal configuration.
* Easy to modify trumps any performance concerns: I don't need it fast!
* Reload the certs when certbot renews them, with no interaction needed for me.
* Somewhat similar proxy passing as nginx proxy_pass directive.

## Example configuration

The [example configuration](example_configuration.json) does roughly this:

1. Listen on port 80, redirecting all requests there to the same host and path, but port 443.
2. Listen on port 443, preparing to serve HTTPS requests there.
3. Log *most stuff* to `/var/log/tjenare.log`
4. For requests to `example.com`...
    1. Load the cert `/etc/letsencrypt/live/example.com/fullchain.pem` and with the key`/etc/letsencrypt/live/example.com/privkey.pem`
    2. Get the subdomain, or use `www` if one is not specified.
    3. Check if the subdomain is defined as a backend and act as a reverse proxy if it is.
    4. If not `spork`, serve the requested file, out of `/var/www/html/{subdomain}/public_html/`

Some examples of requests and where they'll try and read from, given the example configuration:
* `https://foo.example.com/bar.png` = `/var/www/html/example/foo/public_html/bar.png`
* `https://example.com/plop.html` = `/var/www/html/example/www/plop.html`
* `https://horse.example.com/plop/` = `/var/www/html/example/horse/plop/index.html`