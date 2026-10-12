FROM --platform=$BUILDPLATFORM node:22-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ARG VITE_PUBLIC_GATEWAY_URL
ENV VITE_PUBLIC_GATEWAY_URL=$VITE_PUBLIC_GATEWAY_URL
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/templates/default.conf.template
ENV VMCP_CONTROL_PLANE_URL=http://control-plane:8081
ENV NGINX_ENVSUBST_FILTER=^VMCP_CONTROL_PLANE_URL$
ENV NGINX_ENTRYPOINT_QUIET_LOGS=1
RUN rm /etc/nginx/conf.d/default.conf \
    && sed -i '/^user /d; s@^pid .*;@pid /tmp/nginx.pid;@' /etc/nginx/nginx.conf \
    && chown -R 65532:65532 /etc/nginx/conf.d /var/cache/nginx
USER 65532:65532
EXPOSE 8080
COPY --from=build /src/dist /usr/share/nginx/html
