FROM --platform=$BUILDPLATFORM node:26.13.1-alpine@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS build
WORKDIR /src
COPY package.json package-lock.json ./
COPY scripts/install-hooks.mjs ./scripts/install-hooks.mjs
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
ARG VITE_PUBLIC_GATEWAY_URL
ENV VITE_PUBLIC_GATEWAY_URL=$VITE_PUBLIC_GATEWAY_URL
RUN npm run build

FROM nginx:1.31.8-alpine-slim@sha256:4c734d33ee731135dc9f2349914a34551c920f4451fca8bb05bf5880eb9079e8
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
