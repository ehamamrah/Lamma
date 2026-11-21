# syntax=docker/dockerfile:1.7
# Multi-stage: build the SPA with Node, serve it with unprivileged Nginx.

FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:alpine
# Templates are rendered (envsubst) into a tmpfs so the root FS stays read-only.
ENV NGINX_ENVSUBST_OUTPUT_DIR=/tmp
USER root
RUN rm -f /etc/nginx/conf.d/default.conf
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY nginx/proxy-api.inc /etc/nginx/proxy-api.inc
COPY nginx/default.conf.template nginx/security-headers.inc.template /etc/nginx/templates/
COPY --from=build /app/dist /usr/share/nginx/html
USER 101
EXPOSE 8080
STOPSIGNAL SIGQUIT
