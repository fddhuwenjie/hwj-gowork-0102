# 本质评测环境说明

## 项目

- 项目编号：`hwj-gowork-0102`
- 项目名称：承压焊缝无损探伤闭环服务
- 项目说明：基于 Go HTTP JSON 与 SQLite 文件持久化，管理焊缝、设备、校准证书、方法版本、执行批次、探伤报告、异常处置、复核任务、审计记录和后台任务。

## 固定环境

- Go toolchain：`go1.26.5`
- go.mod language version：`go 1.21`
- GOTOOLCHAIN：`local`
- 支持平台：`linux/amd64`、`linux/arm64`
- Docker 基础镜像：`golang:1.26.5-bookworm`
- Docker manifest：`golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd`

## 构建

评测镜像使用仓库内固定的 `benzhi.Dockerfile`，并通过 `build_benzhi_docker.sh` 构建：

```bash
./build_benzhi_docker.sh hwj-gowork-0102:benzhi-amd64 linux/amd64
./build_benzhi_docker.sh hwj-gowork-0102:benzhi-arm64 linux/arm64
```

## 运行

```bash
docker run --rm -it --network none hwj-gowork-0102:benzhi-amd64 bash
```

## 容器内验证

```bash
go version
go env GOTOOLCHAIN GOPROXY GOMODCACHE GOCACHE
go test ./...
go vet ./...
go build ./...
```

---

# 项目 README 同步内容

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
