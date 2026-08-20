# Go 承压焊缝无损探伤闭环服务

本项目提供真实 SQLite 文件持久化的焊缝无损探伤闭环服务，覆盖焊缝、设备、校准证书、方法版本、执行批次、探伤报告、不连续指示、异常事件、返修委托、复核任务、审计记录和后台任务。

## 启动

```bash
export PORT=8080
export DB_PATH=./data/weld-ndt.db
export GOTOOLCHAIN=local
go run ./cmd/server
```

## Docker

构建 linux/amd64 与 linux/arm64：

```bash
chmod +x ./build_docker.sh
IMAGE=weld-ndt:local PLATFORMS=linux/amd64,linux/arm64 ./build_docker.sh
```

## 验收

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
```

## 文档

- `docs/api.md`
- `docs/deploy.md`
- `docs/examples.md`
