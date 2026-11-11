#!/bin/sh
set -eu

cd "$(dirname "$0")/../assets"
HUSKY=0 corepack yarn install --frozen-lockfile
corepack yarn build
cd ..

# 重新生成归档，避免已删除的前端资源残留在旧压缩包中。
rm -f application/statics/assets.zip
zip -qr application/statics/assets.zip assets/build
lec_version=$(node -p "require('./assets/package.json').version")
lec_commit=$(git rev-parse --short HEAD)
go build -trimpath -o lec-drive \
  -ldflags "-X github.com/cloudreve/Cloudreve/v4/application/constants.BackendVersion=$lec_version -X github.com/cloudreve/Cloudreve/v4/application/constants.LastCommit=$lec_commit" .
