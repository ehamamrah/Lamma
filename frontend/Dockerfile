# syntax=docker/dockerfile:1.7
# Multi-stage: build the SPA with Node, serve it with unprivileged Nginx.

FROM node:24-alpine@sha256:116b800c3754b440ee0aa3ed2f20f5bf3d99a554bfd9adeea09ea8017f6e85f5 AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:alpine@sha256:b5047b9b76845eac124358373c6852377c52e4b05834a47647e955dd91cb8f1e
# Templates are rendered (envsubst) into a tmpfs so the root FS stays read-only.
ENV NGINX_ENVSUBST_OUTPUT_DIR=/tmp/nginx
USER root
RUN rm -f /etc/nginx/conf.d/default.conf \
 && printf '#!/bin/sh\nmkdir -p /tmp/nginx\n' > /docker-entrypoint.d/05-tmp-nginx.sh \
 && chmod +x /docker-entrypoint.d/05-tmp-nginx.sh
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY nginx/proxy-api.inc /etc/nginx/proxy-api.inc
COPY nginx/default.conf.template nginx/security-headers.inc.template /etc/nginx/templates/
COPY --from=build /app/dist /usr/share/nginx/html
USER 101
EXPOSE 8080
STOPSIGNAL SIGQUIT
