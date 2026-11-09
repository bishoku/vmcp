FROM --platform=$BUILDPLATFORM node:26.13.0-alpine@sha256:8996bb5821d62a8fd9bb27c1cb07462f3fdce6e47d640c6a881ead0b064ce216 AS build
WORKDIR /src
COPY package.json package-lock.json ./
COPY scripts/install-hooks.mjs ./scripts/install-hooks.mjs
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
ARG VITE_PUBLIC_GATEWAY_URL
ENV VITE_PUBLIC_GATEWAY_URL=$VITE_PUBLIC_GATEWAY_URL
RUN npm run build

FROM nginx:1.31.8-alpine-slim@sha256:9c68ad4f28e7dee4b9484a7859bf451da39ef661bcd038ef62e87593fcd91c86
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
