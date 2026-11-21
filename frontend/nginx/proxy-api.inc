proxy_http_version 1.1;
proxy_set_header Connection "";
proxy_set_header Host $host;
# Overwrite (never append) so clients cannot inject forwarding headers.
proxy_set_header X-Forwarded-For $remote_addr;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Request-ID $request_id;
proxy_connect_timeout 3s;
