# Devtron 从源码检查并构建前后端，不依赖开发机生成的二进制或前端归档。
ARG NODE_IMAGE=node:24.14.1-alpine
ARG GO_IMAGE=golang:1.26.5-alpine
FROM ${NODE_IMAGE} AS frontend
WORKDIR /source/assets
COPY assets/package.json assets/yarn.lock ./
RUN corepack enable && corepack prepare yarn@1.22.22 --activate \
    && HUSKY=0 yarn install --frozen-lockfile --network-timeout 300000
COPY assets/ ./
RUN NODE_OPTIONS=--max-old-space-size=4096 yarn build

FROM ${GO_IMAGE} AS backend
ARG ALPINE_MIRROR=mirrors.ustc.edu.cn
RUN sed -i "s/dl-cdn.alpinelinux.org/${ALPINE_MIRROR}/g" /etc/apk/repositories \
    && apk add --no-cache git zip
WORKDIR /source
ENV GOPROXY=https://goproxy.cn|https://proxy.golang.org|direct CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /source/assets/build ./assets/build
# Go embed 只打包本次前端构建，避免旧文件留在归档中。
RUN rm -f application/statics/assets.zip \
    && zip -qr application/statics/assets.zip assets/build \
    && go test ./pkg/lecsso ./middleware \
    && go build -p 3 -trimpath -o /out/lec-drive \
       -ldflags "-s -w -X github.com/cloudreve/Cloudreve/v4/application/constants.BackendVersion=4.19.1+lec.2" .

FROM ${NODE_IMAGE} AS runtime
ARG ALPINE_MIRROR=mirrors.ustc.edu.cn
RUN sed -i "s/dl-cdn.alpinelinux.org/${ALPINE_MIRROR}/g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata vips-tools
COPY --from=backend /out/lec-drive /usr/local/bin/lec-drive
ENV NODE_ENV=production TZ=Asia/Shanghai CR_SETTING_thumb_vips_enabled=1
WORKDIR /data
USER 1000:1000
EXPOSE 5212
ENTRYPOINT ["/usr/local/bin/lec-drive", "server", "-w", "--conf", "/config/drive.ini"]
