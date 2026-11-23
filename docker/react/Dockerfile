FROM node:24-alpine AS builder
RUN corepack enable
WORKDIR /app
COPY walnut-frontend-react/ ./
RUN pnpm install --frozen-lockfile && pnpm build

FROM nginx:stable-alpine
COPY --from=builder /app/dist/ /usr/share/nginx/html/
COPY docker/config/nginx.conf /etc/nginx/nginx.conf.template
ENV BACKEND_HOST=backend BACKEND_PORT=8011
EXPOSE 80
CMD ["/bin/sh", "-c", "envsubst '$BACKEND_HOST $BACKEND_PORT' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf && exec nginx -g 'daemon off;'"]
