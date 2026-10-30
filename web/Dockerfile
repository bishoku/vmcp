FROM --platform=$BUILDPLATFORM node:26.12.1-alpine@sha256:40cd011b55415bc894e45b02fdd5a5dfde056921b7d23a64c74f80497af48797 AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ARG VITE_PUBLIC_GATEWAY_URL
ENV VITE_PUBLIC_GATEWAY_URL=$VITE_PUBLIC_GATEWAY_URL
RUN npm run build

FROM nginx:1.31.0-alpine-slim@sha256:241b0d0fe06250e026e7a35a008d022c9a1d3bec19442d65cc33b84d0b5dd64d
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
